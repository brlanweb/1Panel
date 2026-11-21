package service

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"time"
)

// Transfer speed tuning for cluster sync.
//
// The slow part of a sync is the master → target HTTP upload over long,
// lossy links (e.g. Oracle Frankfurt → Aliyun Chengdu: ~60-110 kbps with
// ~20% retransmits). Three things help without touching the network:
//
//   1. send fewer bytes: image tarballs are gzipped on the master before
//      the upload (`docker load` accepts gzip as-is);
//   2. keep the pipe full: chunks are uploaded by several concurrent
//      connections. On a lossy path each TCP flow is capped by its own
//      congestion window (cwnd collapses to 1-2 on every loss), so N flows
//      get roughly N× the throughput until the link itself is full;
//   3. bigger initial chunks only when the link is fast (adaptive sizing).
//
// Host-level BBR congestion control (net.ipv4.tcp_congestion_control=bbr)
// helps further on lossy links but is a server change and is left to the
// operator.

var (
	// syncCompressImages gzips image tarballs on the master before upload.
	syncCompressImages = true
	// syncUploadParallel is the number of chunk uploads in flight.
	syncUploadParallel = 4
)

// gzipFile writes src as src+".gz" (fast level: the link, not the CPU, is
// the bottleneck) and reports compressed-input progress.
func gzipFile(ctx context.Context, src string, progress func(done, total int64)) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return "", err
	}
	dst := src + ".gz"
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()
	zw, err := gzip.NewWriterLevel(out, gzip.BestSpeed)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 1<<20)
	var done int64
	last := time.Time{}
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, rErr := in.Read(buf)
		if n > 0 {
			if _, wErr := zw.Write(buf[:n]); wErr != nil {
				return "", wErr
			}
			done += int64(n)
			if progress != nil && (time.Since(last) > time.Second || done == info.Size()) {
				last = time.Now()
				progress(done, info.Size())
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return "", rErr
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	ok = true
	return dst, nil
}

package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fastForkDownload(t *testing.T) {
	oldStall, oldGap := forkDownloadStall, forkDownloadRetryGap
	forkDownloadStall, forkDownloadRetryGap = 2*time.Second, 10*time.Millisecond
	t.Cleanup(func() { forkDownloadStall, forkDownloadRetryGap = oldStall, oldGap })
}

// The first response is cut off mid-body; the retry must resume with a
// Range request instead of starting over, and the result must be intact.
func TestDownloadForkPackageResumesWithRange(t *testing.T) {
	fastForkDownload(t)
	payload := bytes.Repeat([]byte("1panel-fork-package-"), 20000)
	var calls int32
	var sawRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload[:len(payload)/3])
			hj, _ := w.(http.Hijacker)
			conn, buf, _ := hj.Hijack()
			_ = buf.Flush()
			_ = conn.Close()
			return
		}
		sawRange = r.Header.Get("Range")
		var start int
		_, _ = fmt.Sscanf(sawRange, "bytes=%d-", &start)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)-start))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start:])
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "pkg.tar.gz")
	var lastDone, lastTotal int64
	if err := downloadForkPackage(srv.URL, dst, func(d, total int64) { lastDone, lastTotal = d, total }); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload) {
		t.Fatalf("content mismatch: %d vs %d bytes", len(got), len(payload))
	}
	if !strings.HasPrefix(sawRange, "bytes=") || sawRange == "bytes=0-" {
		t.Fatalf("retry did not resume, Range=%q", sawRange)
	}
	if lastDone != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Fatalf("progress %d/%d", lastDone, lastTotal)
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatalf(".part file left behind")
	}
}

func TestDownloadForkPackageStopsOn404(t *testing.T) {
	fastForkDownload(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	if err := downloadForkPackage(srv.URL, filepath.Join(t.TempDir(), "x"), nil); err == nil {
		t.Fatal("404 must fail")
	}
	if calls != 1 {
		t.Fatalf("404 retried %d times, want no retry", calls)
	}
}

func TestVerifyForkPackageChecksum(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	pkg := filepath.Join(t.TempDir(), "p.tar.gz")
	_ = os.WriteFile(pkg, []byte("package-bytes"), 0o600)
	sum := sha256.Sum256([]byte("package-bytes"))
	good := hex.EncodeToString(sum[:])

	body := good
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body + "  1panel-v9.9.9-cluster.1-linux-amd64.tar.gz\n"))
	}))
	defer srv.Close()
	name := "1panel-v9.9.9-cluster.1-linux-amd64.tar.gz"
	withFakeReleases(t, []forkRelease{{TagName: "v9.9.9-cluster.1", Assets: []forkReleaseAsset{
		{Name: name, BrowserDownloadURL: "https://example.invalid/p"},
		{Name: name + ".sha256", BrowserDownloadURL: srv.URL},
	}}})
	if err := verifyForkPackageChecksum("v9.9.9-cluster.1", name, pkg); err != nil {
		t.Fatalf("good checksum rejected: %v", err)
	}
	body = strings.Repeat("0", 64)
	if err := verifyForkPackageChecksum("v9.9.9-cluster.1", name, pkg); err == nil {
		t.Fatal("bad checksum accepted")
	}
}

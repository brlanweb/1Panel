package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
)

// fakeChunkAgent mimics the agent's resumable /files/chunkupload: chunks are
// written at their offset, a retried chunk overwrites the same range.
type fakeChunkAgent struct {
	mu        sync.Mutex
	data      []byte
	requests  int
	dropFirst map[int]bool // chunkIndex -> drop the connection once
	dropped   map[int]bool
	done      bool
	uploadIDs map[string]bool
	// supportsParallel mimics a new agent; received tracks parallel ranges.
	supportsParallel bool
	received         map[int64]int64
	sawParallel      bool
	stopped          []string
	maxInflight      int
	inflight         int
	delay            time.Duration
}

func (f *fakeChunkAgent) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/files/chunkupload/stop" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.stopped = append(f.stopped, body["key"])
			f.mu.Unlock()
			envelope(w, nil)
			return
		}
		if r.URL.Path != "/api/v2/files/chunkupload" {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.inflight++
		if f.inflight > f.maxInflight {
			f.maxInflight = f.inflight
		}
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.inflight--
			f.mu.Unlock()
		}()
		if f.delay > 0 {
			select {
			case <-time.After(f.delay):
			case <-r.Context().Done():
				return
			}
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		index, _ := strconv.Atoi(r.FormValue("chunkIndex"))
		count, _ := strconv.Atoi(r.FormValue("chunkCount"))
		offset, _ := strconv.ParseInt(r.FormValue("offset"), 10, 64)
		size, _ := strconv.ParseInt(r.FormValue("fileSize"), 10, 64)
		file, _, err := r.FormFile("chunk")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		chunk, _ := io.ReadAll(file)

		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		f.uploadIDs[r.FormValue("uploadID")] = true
		if f.dropFirst[index] && !f.dropped[index] {
			f.dropped[index] = true
			// Simulate the slow-link failure: the connection is torn down
			// before any response is written.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("hijack not supported")
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
		if int64(len(f.data)) < size {
			grown := make([]byte, size)
			copy(grown, f.data)
			f.data = grown
		}
		copy(f.data[offset:], chunk)
		if f.supportsParallel && r.FormValue("parallel") == "1" {
			f.sawParallel = true
			if f.received == nil {
				f.received = map[int64]int64{}
			}
			f.received[offset] = int64(len(chunk))
			var got int64
			for _, l := range f.received {
				got += l
			}
			if got == size {
				f.done = true
			}
			envelope(w, map[string]bool{"parallel": true})
			return
		}
		if index+1 == count {
			if offset+int64(len(chunk)) != size {
				http.Error(w, "final chunk does not match file size", http.StatusBadRequest)
				return
			}
			f.done = true
		}
		envelope(w, true)
	}
}

func withSmallChunks(t *testing.T) {
	oldMin, oldStart, oldMax, oldGap := syncChunkMin, syncChunkStart, syncChunkMax, syncChunkRetryGap
	syncChunkMin, syncChunkStart, syncChunkMax, syncChunkRetryGap = 1024, 4096, 16384, time.Millisecond
	t.Cleanup(func() {
		syncChunkMin, syncChunkStart, syncChunkMax, syncChunkRetryGap = oldMin, oldStart, oldMax, oldGap
	})
}

func writeTestPayload(t *testing.T, size int) (string, []byte) {
	t.Helper()
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i*31 + i/7)
	}
	tmp := filepath.Join(t.TempDir(), "payload.tar")
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return tmp, payload
}

func TestSyncChunkUploadRetriesDroppedChunk(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	withSmallChunks(t)

	agent := &fakeChunkAgent{dropFirst: map[int]bool{0: true, 3: true}, dropped: map[int]bool{}, uploadIDs: map[string]bool{}}
	srv := httptest.NewServer(agent.handler(t))
	defer srv.Close()
	node := insertTestNode(t, "slow", srv.URL, model.ClusterNodeStatusOnline)

	tmp, payload := writeTestPayload(t, 100_000)
	var lastSent, lastTotal int64
	err := NewIClusterSyncService().(*ClusterSyncService).uploadRemoteFile(context.Background(), node, "/tmp/x", "payload.tar", tmp, true, func(sent, total int64) {
		lastSent, lastTotal = sent, total
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !agent.done || !bytes.Equal(agent.data, payload) {
		t.Fatalf("target content mismatch (done=%v, len=%d)", agent.done, len(agent.data))
	}
	if !agent.dropped[0] || !agent.dropped[3] {
		t.Fatalf("expected dropped chunks to be retried, dropped=%v", agent.dropped)
	}
	if len(agent.uploadIDs) != 1 {
		t.Fatalf("one transfer must use a single uploadID, got %v", agent.uploadIDs)
	}
	if lastSent != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Fatalf("progress = %d/%d, want %d", lastSent, lastTotal, len(payload))
	}
}

func TestSyncChunkUploadFallsBackWhenUnsupported(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	withSmallChunks(t)

	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/files/upload" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseMultipartForm(64 << 20)
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got, _ = io.ReadAll(file)
		envelope(w, nil)
	}))
	defer srv.Close()
	node := insertTestNode(t, "old", srv.URL, model.ClusterNodeStatusOnline)

	tmp, payload := writeTestPayload(t, 20_000)
	if err := NewIClusterSyncService().(*ClusterSyncService).uploadRemoteFile(context.Background(), node, "/tmp/x", "payload.tar", tmp, true, nil); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("fallback upload content mismatch (%d bytes)", len(got))
	}
}

func TestSyncByteProgressReportsDoneTotalAndRate(t *testing.T) {
	var gotDone, gotTotal, gotRate int64
	calls := 0
	opts := syncTransferOpts{Progress: func(done, total, rate int64) {
		calls++
		gotDone, gotTotal, gotRate = done, total, rate
	}}
	cb := opts.byteProgress()
	cb(10, 1000) // first call always reported (no previous report)
	cb(20, 1000) // throttled
	time.Sleep(5 * time.Millisecond)
	cb(1000, 1000) // completion is always reported
	if calls != 2 {
		t.Fatalf("progress reported %d times, want 2 (throttled)", calls)
	}
	if gotDone != 1000 || gotTotal != 1000 || gotRate <= 0 {
		t.Fatalf("final progress = %d/%d rate %d", gotDone, gotTotal, gotRate)
	}
	if (syncTransferOpts{}).byteProgress() != nil {
		t.Fatalf("no Progress sink must yield a nil callback")
	}
}

// New agents accept out-of-order chunks: the master uploads with several
// connections at once and the file is still byte-identical.
func TestSyncChunkUploadParallelWithNewAgent(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	withSmallChunks(t)
	agent := &fakeChunkAgent{supportsParallel: true, dropFirst: map[int]bool{5: true}, dropped: map[int]bool{}, uploadIDs: map[string]bool{}, delay: 20 * time.Millisecond}
	srv := httptest.NewServer(agent.handler(t))
	defer srv.Close()
	node := insertTestNode(t, "fast", srv.URL, model.ClusterNodeStatusOnline)

	tmp, payload := writeTestPayload(t, 150_000)
	var last int64
	err := NewIClusterSyncService().(*ClusterSyncService).uploadRemoteFile(context.Background(), node, "/tmp/x", "payload.tar", tmp, true, func(sent, total int64) {
		last = sent
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !agent.sawParallel || agent.maxInflight < 2 {
		t.Fatalf("expected parallel upload, sawParallel=%v maxInflight=%d", agent.sawParallel, agent.maxInflight)
	}
	if !agent.done || !bytes.Equal(agent.data, payload) {
		t.Fatalf("target content mismatch")
	}
	if !agent.dropped[5] {
		t.Fatalf("dropped chunk was not retried")
	}
	if last != int64(len(payload)) {
		t.Fatalf("progress %d, want %d", last, len(payload))
	}
	if len(agent.stopped) != 0 {
		t.Fatalf("successful upload must not discard the target file")
	}
}

// Cancelling mid-upload stops promptly and asks the target to discard the
// partial file.
func TestSyncChunkUploadCancelCleansTarget(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	withSmallChunks(t)
	agent := &fakeChunkAgent{supportsParallel: true, dropped: map[int]bool{}, uploadIDs: map[string]bool{}, delay: 50 * time.Millisecond}
	srv := httptest.NewServer(agent.handler(t))
	defer srv.Close()
	node := insertTestNode(t, "slow", srv.URL, model.ClusterNodeStatusOnline)
	tmp, _ := writeTestPayload(t, 400_000)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(150*time.Millisecond, cancel)
	started := time.Now()
	err := NewIClusterSyncService().(*ClusterSyncService).uploadRemoteFile(ctx, node, "/tmp/x", "payload.tar", tmp, true, nil)
	if !errors.Is(err, errSyncCancelled) {
		t.Fatalf("err = %v, want cancelled", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("cancel took %s", time.Since(started))
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.stopped) != 1 || !agent.uploadIDs[agent.stopped[0]] {
		t.Fatalf("target partial upload not discarded: stopped=%v ids=%v", agent.stopped, agent.uploadIDs)
	}
	if agent.done {
		t.Fatalf("cancelled upload must not complete")
	}
}

func TestGzipFileRoundTrip(t *testing.T) {
	src := filepath.Join(t.TempDir(), "image.tar")
	payload := bytes.Repeat([]byte("docker-layer-data "), 50_000)
	_ = os.WriteFile(src, payload, 0o600)
	gz, err := gzipFile(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(gz)
	if info.Size() >= int64(len(payload))/5 {
		t.Fatalf("compressed %d of %d bytes, expected strong compression", info.Size(), len(payload))
	}
	f, _ := os.Open(gz)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(zr)
	if !bytes.Equal(out, payload) {
		t.Fatal("round trip mismatch")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gzipFile(ctx, src, nil); err == nil {
		t.Fatal("cancelled gzip must fail")
	}
	if _, err := os.Stat(src + ".gz.tmp"); !os.IsNotExist(err) {
		_ = err
	}
}

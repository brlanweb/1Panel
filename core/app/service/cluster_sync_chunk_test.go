package service

import (
	"bytes"
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
}

func (f *fakeChunkAgent) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/files/chunkupload" {
			http.NotFound(w, r)
			return
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
	err := NewIClusterSyncService().(*ClusterSyncService).uploadRemoteFile(node, "/tmp/x", "payload.tar", tmp, true, func(sent, total int64) {
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
	if err := NewIClusterSyncService().(*ClusterSyncService).uploadRemoteFile(node, "/tmp/x", "payload.tar", tmp, true, nil); err != nil {
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

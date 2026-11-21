package v2

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Chunks of a parallel upload arrive out of order (and one twice, as a
// retry); the file is finalized only once every byte is present and is
// byte-identical to the source.
func TestWriteResumableUploadChunkParallelOutOfOrder(t *testing.T) {
	dir := t.TempDir()
	payload := bytes.Repeat([]byte("0123456789abcdef"), 1000) // 16000 bytes
	const size = 4000
	order := []int{2, 0, 3, 1, 1}
	uploadID := "cluster-sync-test-parallel"
	t.Cleanup(func() { forgetParallelChunks(uploadID); deleteActiveChunkUpload(uploadID) })
	dst := filepath.Join(dir, "image.tar.gz")
	for i, idx := range order {
		off := int64(idx * size)
		err := writeResumableUploadChunk(resumableUploadChunk{
			UploadID: uploadID, Filename: "image.tar.gz", DstDir: dir,
			ChunkIndex: idx, ChunkCount: 4, Offset: off, FileSize: int64(len(payload)),
			Overwrite: true, Parallel: true,
		}, payload[off:off+size])
		if err != nil {
			t.Fatalf("chunk %d (step %d): %v", idx, i, err)
		}
		_, statErr := os.Stat(dst)
		complete := i >= 3
		if complete != (statErr == nil) {
			t.Fatalf("after step %d file exists=%v, want %v", i, statErr == nil, complete)
		}
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("content mismatch")
	}
	if _, err := os.Stat(resumableUploadPartPath(dir, uploadID)); !os.IsNotExist(err) {
		t.Fatalf("part file left behind")
	}
}

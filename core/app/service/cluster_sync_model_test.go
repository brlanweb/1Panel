package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/global"
)

// fakeModelTree serves /api/v2/files/search and /api/v2/files/download for
// an in-memory directory tree (path -> content), like the real agent.
func fakeModelTree(files map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			var body struct {
				Path   string `json:"path"`
				Search string `json:"search"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			dir := path.Clean(body.Path)
			seen := map[string]bool{}
			var items []map[string]interface{}
			exists := false
			for p, content := range files {
				if !strings.HasPrefix(p, dir+"/") {
					continue
				}
				exists = true
				rest := strings.TrimPrefix(p, dir+"/")
				name := strings.SplitN(rest, "/", 2)[0]
				if seen[name] || (body.Search != "" && !strings.Contains(name, body.Search)) {
					continue
				}
				seen[name] = true
				if strings.Contains(rest, "/") {
					items = append(items, map[string]interface{}{"name": name, "isDir": true})
				} else {
					items = append(items, map[string]interface{}{"name": name, "isDir": false, "size": len(content)})
				}
			}
			if !exists {
				envelope(w, map[string]interface{}{"path": "", "items": []interface{}{}})
				return
			}
			envelope(w, map[string]interface{}{"path": dir, "itemTotal": len(items), "items": items})
		case "/api/v2/files/download":
			content, ok := files[r.URL.Query().Get("path")]
			if !ok {
				envelope(w, nil)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeContent(w, r, "f", time.Time{}, strings.NewReader(content))
		default:
			http.NotFound(w, r)
		}
	}
}

func TestSyncModelSend(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	oldDir := global.CONF.Base.InstallDir
	global.CONF.Base.InstallDir = t.TempDir()
	defer func() { global.CONF.Base.InstallDir = oldDir }()

	root := "/opt/1panel/ai/vllm/models/Qwen/Qwen3-0.6B"
	source := map[string]string{
		root + "/config.json":                  `{"a":1}`,
		root + "/sub/model.safetensors":        strings.Repeat("w", 3000),
		root + "/.cache/huggingface/x.lock":    "skip-me",
		root + "/sub/._____temp_partial.bin":   "skip-me",
		"/opt/1panel/ai/vllm/models/other.txt": "not part of the model",
	}
	local := fakeModelTree(source)
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ai/vllm/models/search" {
			envelope(w, map[string]interface{}{"total": 2, "items": []map[string]interface{}{
				{"id": 1, "repoId": "Qwen/Qwen3-0.6B", "source": "modelscope", "kind": "model", "dir": root, "status": "Success", "size": 3007},
				{"id": 2, "repoId": "Qwen/Downloading", "source": "modelscope", "kind": "model", "dir": "/x", "status": "Downloading"},
			}})
			return
		}
		local(w, r)
	}))

	var mu sync.Mutex
	uploaded := map[string]string{}
	var registered map[string]interface{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"path": "", "items": []interface{}{}})
		case "/api/v2/files/upload":
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			f, hdr, _ := r.FormFile("file")
			raw, _ := io.ReadAll(f)
			mu.Lock()
			uploaded[path.Join(r.FormValue("path"), hdr.Filename)] = string(raw)
			mu.Unlock()
			envelope(w, map[string]interface{}{})
		case "/api/v2/ai/vllm/models/import":
			_ = json.NewDecoder(r.Body).Decode(&registered)
			envelope(w, nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	target := insertTestNode(t, "gpu-node", remote.URL, "online")

	svc := NewIClusterSyncService()
	list, err := svc.ListResources(dto.ClusterSyncResourceSearch{Kind: "model", SourceNodeID: 0})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(list) != 1 || list[0].Key != root {
		t.Fatalf("models = %+v, want only the finished model", list)
	}

	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{Kind: "model", SourceNodeID: 0, TargetNodeIDs: []uint{target.ID}, ResourceKeys: []string{root}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	records, _ := svc.TaskRecords(taskID)
	if len(records) != 1 || records[0].Action != "create" {
		t.Fatalf("records = %+v, want one create", records)
	}
	want := map[string]string{
		root + "/config.json":           `{"a":1}`,
		root + "/sub/model.safetensors": strings.Repeat("w", 3000),
	}
	if len(uploaded) != len(want) {
		t.Fatalf("uploaded %d files %v, want %d (caches / partial files skipped)", len(uploaded), keys(uploaded), len(want))
	}
	for p, content := range want {
		if uploaded[p] != content {
			t.Errorf("target %s = %d bytes, want %d at the same path", p, len(uploaded[p]), len(content))
		}
	}
	if registered["dir"] != root || registered["repoId"] != "Qwen/Qwen3-0.6B" || registered["source"] != "modelscope" || registered["kind"] != "model" {
		t.Errorf("import body = %v, want the model registered at %s", registered, root)
	}
	if entries, _ := os.ReadDir(modelSyncTmpDir()); len(entries) != 0 {
		t.Errorf("master temp dir not cleaned: %d entries left", len(entries))
	}
}

func TestSyncModelSkip(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	oldDir := global.CONF.Base.InstallDir
	global.CONF.Base.InstallDir = t.TempDir()
	defer func() { global.CONF.Base.InstallDir = oldDir }()

	root := "/models/m"
	files := map[string]string{root + "/config.json": "{}", root + "/w.bin": "weights"}
	startLocalAgentTestServer(t, fakeModelTree(files))
	targetTree := fakeModelTree(files) // target already has every file
	uploads := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/upload", "/api/v2/files/chunkupload":
			uploads++
			envelope(w, nil)
		case "/api/v2/ai/vllm/models/import":
			envelope(w, nil)
		default:
			targetTree(w, r)
		}
	}))
	defer remote.Close()
	target := insertTestNode(t, "n2", remote.URL, "online")

	svc := &ClusterSyncService{nodes: NewIClusterNodeService().(*ClusterNodeService)}
	action, reason := svc.transferModel(localClusterNode(), target, dto.ClusterSyncResource{Key: root, Name: "org/m", Detail: "model"}, syncTransferOpts{})
	if action != "skip-same" || uploads != 0 {
		t.Fatalf("action %s (%s), uploads %d; want skip-same without uploads", action, reason, uploads)
	}
}

// A connection dropped mid-file is resumed with a Range request instead of
// restarting the file.
func TestDownloadModelFileResumesWithRange(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	content := bytes.Repeat([]byte("0123456789"), 20000) // 200 KB
	var mu sync.Mutex
	var ranges []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Range"))
		first := len(ranges) == 1
		mu.Unlock()
		w.Header().Set("Content-Type", "application/octet-stream")
		if first {
			w.Header().Set("Content-Length", "200000")
			_, _ = w.Write(content[:50000])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler) // drop the connection mid-body
		}
		http.ServeContent(w, r, "f", time.Time{}, bytes.NewReader(content))
	}))
	defer remote.Close()
	node := insertTestNode(t, "src", remote.URL, "online")

	svc := &ClusterSyncService{nodes: NewIClusterNodeService().(*ClusterNodeService)}
	tmp, err := svc.downloadModelFile(context.Background(), node, "/m/w.bin", int64(len(content)), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("downloadModelFile: %v", err)
	}
	got, _ := os.ReadFile(tmp)
	if !bytes.Equal(got, content) {
		t.Fatalf("downloaded %d bytes, content mismatch", len(got))
	}
	if len(ranges) < 2 || ranges[0] != "" || !strings.HasPrefix(ranges[1], "bytes=") || ranges[1] == "bytes=0-" {
		t.Fatalf("Range headers = %v, want a resumed request from the received offset", ranges)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

package service

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupSyncTestDB opens an in-memory SQLite DB, points global.DB at it, and
// seeds the test encryption key through global.CONF so node API keys can be
// encrypted/decrypted without a settings table. It returns a restore func.
func setupSyncTestDB(t *testing.T) func() {
	t.Helper()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.ClusterNode{}, &model.ClusterSyncTask{}, &model.ClusterSyncRecord{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	global.DB = db
	global.CONF.Base.EncryptKey = "test-encrypt-key-for-cluster-sync"
	oldAsync := syncExecuteAsync
	syncExecuteAsync = false
	return func() {
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
		syncExecuteAsync = oldAsync
	}
}

// The agent only lists directory children when "expand" is true; this
// fake answers exactly like the real agent (empty items otherwise), so a
// missing flag fails the test instead of passing on a too-friendly mock.
func TestSyncFileListSendsExpandAndReturnsDirs(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if expand, _ := body["expand"].(bool); !expand {
			envelope(w, map[string]interface{}{"path": "/data", "isDir": true, "items": nil})
			return
		}
		envelope(w, map[string]interface{}{"path": "/data", "isDir": true, "items": []map[string]interface{}{
			{"name": "a.txt", "isDir": false, "size": 10},
			{"name": "sub", "isDir": true},
		}})
	}))
	res, err := NewIClusterSyncService().ListResources(dto.ClusterSyncResourceSearch{Kind: "file", SourceNodeID: 0, Path: "/data"})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(res) != 2 || res[0].Key != "/data/sub" || res[0].Extra != syncResourceDir || res[1].Key != "/data/a.txt" {
		t.Fatalf("resources = %+v, want dir /data/sub then file /data/a.txt", res)
	}
}

func TestSyncAppListReadsAgentAppDetailID(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": 1, "items": []map[string]interface{}{
			{"id": 7, "name": "redis", "appKey": "redis", "version": "7.2.5", "appDetailID": 42},
		}})
	}))
	res, err := NewIClusterSyncService().ListResources(dto.ClusterSyncResourceSearch{Kind: "app", SourceNodeID: 0})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(res) != 1 || res[0].Extra != "42" {
		t.Fatalf("resources = %+v, want appDetailID 42 in Extra", res)
	}
}

func insertTestNode(t *testing.T, name, entryURL, status string) model.ClusterNode {
	t.Helper()
	node := model.ClusterNode{Name: name, EntryURL: entryURL, Status: status}
	if err := global.DB.Create(&node).Error; err != nil {
		t.Fatalf("create test node: %v", err)
	}
	cipher, err := encrypt.EncryptAPIKey("test-api-key", strconv.FormatUint(uint64(node.ID), 10), global.CONF.Base.EncryptKey)
	if err != nil {
		t.Fatalf("encrypt test api key: %v", err)
	}
	if err := global.DB.Model(&model.ClusterNode{}).Where("id = ?", node.ID).Update("api_key_cipher", cipher).Error; err != nil {
		t.Fatalf("store test cipher: %v", err)
	}
	node.APIKeyCipher = cipher
	return node
}

// envelope writes a 1Panel-style {code, message, data} response.
func envelope(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "message": "success", "data": data})
}

func TestSyncPreviewImage(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	sourceImages := []map[string]interface{}{
		{"id": "sha256:aaa", "tags": []string{"redis:7.2.5"}, "size": 100},
		{"id": "sha256:bbb", "tags": []string{"mysql:8.0.37"}, "size": 200},
		{"id": "sha256:ccc", "tags": []string{"postgres:16.3"}, "size": 300},
	}
	targetImages := []map[string]interface{}{
		{"id": "sha256:aaa", "tags": []string{"redis:7.2.5"}, "size": 100},
		{"id": "sha256:xxx", "tags": []string{"mysql:8.0.37"}, "size": 200},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/containers/image/search", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": len(sourceImages), "items": sourceImages})
	})
	source := httptest.NewServer(mux)
	defer source.Close()
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/api/v2/containers/image/search", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": len(targetImages), "items": targetImages})
	})
	target := httptest.NewServer(mux2)
	defer target.Close()

	sourceNode := insertTestNode(t, "source", source.URL, model.ClusterNodeStatusOnline)
	targetNode := insertTestNode(t, "target", target.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterSyncService()
	items, err := svc.Preview(dto.ClusterSyncPreviewReq{
		Kind:          dto.ClusterSyncKindImage,
		SourceNodeID:  sourceNode.ID,
		TargetNodeIDs: []uint{targetNode.ID},
		ResourceKeys:  []string{"redis:7.2.5", "mysql:8.0.37", "postgres:16.3"},
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("Preview returned %d items, want 3", len(items))
	}
	want := map[string]string{
		"redis:7.2.5":   model.ClusterSyncActionSkipSame,
		"mysql:8.0.37":  model.ClusterSyncActionSkipConflict,
		"postgres:16.3": model.ClusterSyncActionCreate,
	}
	for _, item := range items {
		if want[item.ResourceKey] != item.Action {
			t.Errorf("Preview(%s) = %s (%s), want %s", item.ResourceKey, item.Action, item.Reason, want[item.ResourceKey])
		}
		if item.TargetNodeID != targetNode.ID {
			t.Errorf("Preview(%s) target = %d, want %d", item.ResourceKey, item.TargetNodeID, targetNode.ID)
		}
	}
}

func TestAggregateSnapshot(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/dashboard/current/node", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{
			"cpuUsedPercent": 12.5, "cpuTotal": 4,
			"memoryUsedPercent": 30.0, "memoryTotal": 8000000000,
		})
	})
	mux.HandleFunc("/api/v2/apps/installed/search", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": 3, "items": []interface{}{}})
	})
	mux.HandleFunc("/api/v2/websites/search", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": 2, "items": []interface{}{}})
	})
	mux.HandleFunc("/api/v2/health/check", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	node := insertTestNode(t, "node1", server.URL, model.ClusterNodeStatusOnline)
	svc := NewIClusterNodeService()
	overview, err := svc.Aggregate()
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if overview.TotalApps != 3 || overview.TotalWebsites != 2 {
		t.Errorf("Aggregate totals = apps %d websites %d, want 3 and 2", overview.TotalApps, overview.TotalWebsites)
	}
	if len(overview.Nodes) != 1 {
		t.Fatalf("Aggregate nodes = %d, want 1", len(overview.Nodes))
	}
	snap := overview.Nodes[0]
	if snap.CpuUsedPercent != 12.5 || snap.MemUsedPercent != 30.0 || snap.AppCount != 3 || snap.WebsiteCount != 2 {
		t.Errorf("snapshot = %+v, want cpu 12.5 mem 30 apps 3 websites 2", snap)
	}
	if snap.FetchError != "" {
		t.Errorf("snapshot FetchError = %q, want empty", snap.FetchError)
	}
	// The snapshot must also be persisted on the node row.
	var persisted model.ClusterNode
	if err := global.DB.First(&persisted, node.ID).Error; err != nil {
		t.Fatalf("read persisted node: %v", err)
	}
	if persisted.AppCount != 3 || persisted.WebsiteCount != 2 {
		t.Errorf("persisted counts = apps %d websites %d, want 3 and 2", persisted.AppCount, persisted.WebsiteCount)
	}
}

func TestSyncPreviewPhp(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	sourceTemplates := []map[string]interface{}{
		{"id": 1, "name": "WordPress", "extensions": "exif,igbinary,imagick"},
		{"id": 2, "name": "SeaCMS", "extensions": "mysqli,pdo_mysql,gd,curl"},
		{"id": 3, "name": "Custom", "extensions": "gd,zip"},
	}
	targetTemplates := []map[string]interface{}{
		{"id": 1, "name": "WordPress", "extensions": "exif,igbinary,imagick"},
		{"id": 2, "name": "SeaCMS", "extensions": "mysqli,pdo_mysql,gd"},
	}
	phpHandler := func(items []map[string]interface{}) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			envelope(w, map[string]interface{}{"total": len(items), "items": items})
		}
	}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/runtimes/php/extensions/search" {
			phpHandler(sourceTemplates)(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer source.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/runtimes/php/extensions/search" {
			phpHandler(targetTemplates)(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer target.Close()

	sourceNode := insertTestNode(t, "source", source.URL, model.ClusterNodeStatusOnline)
	targetNode := insertTestNode(t, "target", target.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterSyncService()
	items, err := svc.Preview(dto.ClusterSyncPreviewReq{
		Kind:          dto.ClusterSyncKindPhp,
		SourceNodeID:  sourceNode.ID,
		TargetNodeIDs: []uint{targetNode.ID},
		ResourceKeys:  []string{"1", "2", "3"},
	})
	if err != nil {
		t.Fatalf("Preview php: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("Preview returned %d items, want 3", len(items))
	}
	want := map[string]string{
		"1": model.ClusterSyncActionSkipSame,
		"2": model.ClusterSyncActionSkipConflict,
		"3": model.ClusterSyncActionCreate,
	}
	for _, item := range items {
		if want[item.ResourceKey] != item.Action {
			t.Errorf("Preview(%s) = %s (%s), want %s", item.ResourceKey, item.Action, item.Reason, want[item.ResourceKey])
		}
	}
}

func TestSyncImageSearchSendsOrderBy(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	var gotBody map[string]interface{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/containers/image/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		envelope(w, map[string]interface{}{"total": 0, "items": []interface{}{}})
	})
	source := httptest.NewServer(mux)
	defer source.Close()
	sourceNode := insertTestNode(t, "source", source.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterSyncService()
	if _, err := svc.ListResources(dto.ClusterSyncResourceSearch{
		Kind: dto.ClusterSyncKindImage, SourceNodeID: sourceNode.ID, Page: 1, PageSize: 10,
	}); err != nil {
		t.Fatalf("ListResources image: %v", err)
	}
	// PageImage requires orderBy + order; omitting them used to be a
	// backend 400, which made the image list silently empty.
	if gotBody["orderBy"] != "createdAt" || gotBody["order"] != "null" {
		t.Errorf("image search body = %v, want orderBy=createdAt order=null", gotBody)
	}
}

func TestSyncPhpImport(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	// Fake target: import creates, returns "skipped" for identical content,
	// and a conflict error for a same-name template with other extensions.
	imported := map[string]string{"WordPress": "exif,igbinary,imagick"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/runtimes/php/extensions/search", func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]interface{}{}
		for name, ext := range imported {
			items = append(items, map[string]interface{}{"id": 1, "name": name, "extensions": ext})
		}
		envelope(w, map[string]interface{}{"total": len(items), "items": items})
	})
	mux.HandleFunc("/api/v2/runtimes/php/extensions/import", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		name, _ := body["name"].(string)
		ext, _ := body["extensions"].(string)
		if old, ok := imported[name]; ok {
			if old == ext {
				envelope(w, "skipped")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 500, "message": "ErrPHPExtensionsConflict", "data": nil})
			return
		}
		imported[name] = ext
		envelope(w, "created")
	})
	target := httptest.NewServer(mux)
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": 0, "items": []interface{}{}})
	}))
	defer source.Close()
	sourceNode := insertTestNode(t, "source", source.URL, model.ClusterNodeStatusOnline)
	targetNode := insertTestNode(t, "target", target.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterSyncService().(*ClusterSyncService)
	// Same content -> skipped by the remote import itself.
	action, _ := svc.transferOne(dto.ClusterSyncKindPhp, sourceNode, targetNode,
		dto.ClusterSyncResource{Key: "9", Name: "WordPress", Detail: "exif,igbinary,imagick"}, "", false)
	if action != model.ClusterSyncActionSkipSame {
		t.Errorf("transferOne same php template = %s, want skip-same", action)
	}
	// Different extensions, no overwrite -> skip-conflict.
	action, _ = svc.transferOne(dto.ClusterSyncKindPhp, sourceNode, targetNode,
		dto.ClusterSyncResource{Key: "9", Name: "WordPress", Detail: "gd,zip"}, "", false)
	if action != model.ClusterSyncActionSkipConflict {
		t.Errorf("transferOne conflicting php template = %s, want skip-conflict", action)
	}
	// New template -> created.
	action, _ = svc.transferOne(dto.ClusterSyncKindPhp, sourceNode, targetNode,
		dto.ClusterSyncResource{Key: "10", Name: "SeaCMS", Detail: "mysqli,pdo_mysql,gd,curl"}, "", false)
	if action != model.ClusterSyncActionCreate {
		t.Errorf("transferOne new php template = %s, want create", action)
	}
}

// startLocalAgentTestServer serves handler on a temp unix socket and points
// the sync service's local agent client at it, mimicking this panel's own
// agent (the unix socket is the trust boundary, no API-key auth).
func startLocalAgentTestServer(t *testing.T, handler http.Handler) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	old := localAgentSocketPath
	localAgentSocketPath = sock
	t.Cleanup(func() { localAgentSocketPath = old })
}

func TestSyncLocalNodeListsFiles(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/files/search" {
			http.NotFound(w, r)
			return
		}
		envelope(w, map[string]interface{}{"items": []map[string]interface{}{
			{"name": "a.txt", "isDir": false, "size": 10},
			{"name": "dir", "isDir": true},
		}})
	}))
	svc := NewIClusterSyncService()
	res, err := svc.ListResources(dto.ClusterSyncResourceSearch{Kind: "file", SourceNodeID: 0, Path: "/data", Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("ListResources local file: %v", err)
	}
	if len(res) != 2 || res[0].Extra != syncResourceDir || res[1].Key != "/data/a.txt" {
		t.Fatalf("resources = %+v, want dir first then file /data/a.txt", res)
	}
}

func TestSyncLocalNodeListsImages(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/containers/image/search" {
			http.NotFound(w, r)
			return
		}
		envelope(w, map[string]interface{}{
			"total": 1,
			"items": []map[string]interface{}{{"id": "sha256:aaa", "tags": []string{"redis:7.2.5"}, "size": 100}},
		})
	}))
	svc := NewIClusterSyncService()
	res, err := svc.ListResources(dto.ClusterSyncResourceSearch{Kind: "image", SourceNodeID: 0, Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("ListResources local image: %v", err)
	}
	if len(res) != 1 || res[0].Key != "redis:7.2.5" {
		t.Fatalf("resources = %+v, want one image redis:7.2.5", res)
	}
}

func TestSyncLocalToRemoteFileExecute(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []map[string]interface{}{
				{"name": "a.txt", "isDir": false, "size": 11},
			}})
		case "/api/v2/files/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("hello-sync!"))
		default:
			http.NotFound(w, r)
		}
	}))
	var uploadedName, uploadedDir, uploadedBytes string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []interface{}{}})
		case "/api/v2/files/upload":
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			uploadedDir = r.FormValue("path")
			f, hdr, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer f.Close()
			uploadedName = hdr.Filename
			raw, _ := io.ReadAll(f)
			uploadedBytes = string(raw)
			envelope(w, map[string]interface{}{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	target := insertTestNode(t, "node-b", remote.URL, "online")

	svc := NewIClusterSyncService()
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "file", SourceNodeID: 0, TargetNodeIDs: []uint{target.ID},
		ResourceKeys: []string{"/data/a.txt"}, Path: "/data",
	})
	if err != nil {
		t.Fatalf("Execute local->remote file: %v", err)
	}
	if taskID == 0 {
		t.Fatal("Execute returned taskID 0")
	}
	if uploadedName != "a.txt" || uploadedDir != "/data" || uploadedBytes != "hello-sync!" {
		t.Errorf("upload = name %q dir %q bytes %q, want a.txt /data hello-sync!", uploadedName, uploadedDir, uploadedBytes)
	}
	records, err := svc.TaskRecords(taskID)
	if err != nil {
		t.Fatalf("TaskRecords: %v", err)
	}
	if len(records) != 1 || records[0].Action != "create" {
		t.Errorf("records = %+v, want one create record", records)
	}
}

func TestSyncRemoteToLocalFileExecute(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []map[string]interface{}{
				{"name": "b.txt", "isDir": false, "size": 5},
			}})
		case "/api/v2/files/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("b-data"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	source := insertTestNode(t, "node-b", remote.URL, "online")
	var uploadedName, uploadedBytes string
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []interface{}{}})
		case "/api/v2/files/upload":
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			f, hdr, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer f.Close()
			uploadedName = hdr.Filename
			raw, _ := io.ReadAll(f)
			uploadedBytes = string(raw)
			envelope(w, map[string]interface{}{})
		default:
			http.NotFound(w, r)
		}
	}))

	svc := NewIClusterSyncService()
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "file", SourceNodeID: source.ID, TargetNodeIDs: []uint{0},
		ResourceKeys: []string{"/data/b.txt"}, Path: "/data",
	})
	if err != nil {
		t.Fatalf("Execute remote->local file: %v", err)
	}
	if uploadedName != "b.txt" || uploadedBytes != "b-data" {
		t.Errorf("local upload = name %q bytes %q, want b.txt b-data", uploadedName, uploadedBytes)
	}
	records, err := svc.TaskRecords(taskID)
	if err != nil {
		t.Fatalf("TaskRecords: %v", err)
	}
	if len(records) != 1 || records[0].Action != "create" {
		t.Errorf("records = %+v, want one create record", records)
	}
}

func TestSyncFileListParsesRealAgentShape(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	// The real agent answers /api/v2/files/search with response.FileInfo
	// (files.FileInfo): entries live under "items", with path/name at the
	// top level. The old parser read a "files" key that never exists, so
	// the list was always empty on a real panel while the tests faked the
	// wrong shape and passed.
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/files/search" {
			http.NotFound(w, r)
			return
		}
		envelope(w, map[string]interface{}{
			"path": "/data", "name": "data", "itemTotal": 3,
			"items": []map[string]interface{}{
				{"name": "a.txt", "isDir": false, "size": 11},
				{"name": "sub", "isDir": true, "size": 0},
				{"name": "b.log", "isDir": false, "size": 5},
			},
		})
	}))
	svc := NewIClusterSyncService()
	res, err := svc.ListResources(dto.ClusterSyncResourceSearch{Kind: "file", SourceNodeID: 0, Path: "/data", Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("ListResources local file: %v", err)
	}
	if len(res) != 3 || res[0].Key != "/data/sub" || res[0].Extra != syncResourceDir ||
		res[1].Key != "/data/a.txt" || res[1].Size != 11 || res[2].Key != "/data/b.log" || res[2].Size != 5 {
		t.Fatalf("resources = %+v, want dir /data/sub, /data/a.txt (11), /data/b.log (5)", res)
	}
}

// fileTransferFixture wires a local fake agent (unix socket) and a remote
// fake panel (httptest) for file sync tests. content is what the source
// download returns; targetSize is the size the target already has for the
// same name (-1 = target has no such file). It returns the service and
// pointers to the captured upload (bytes, name, dir, call count).
func fileTransferFixture(t *testing.T, content []byte, targetSize int64) (IClusterSyncService, model.ClusterNode, *[]byte, *string, *string, *int) {
	t.Helper()
	restore := setupSyncTestDB(t)
	t.Cleanup(restore)
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []map[string]interface{}{
				{"name": "a.txt", "isDir": false, "size": len(content)},
			}})
		case "/api/v2/files/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	var uploaded []byte
	var uploadedName, uploadedDir string
	uploadCount := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			items := []interface{}{}
			if targetSize >= 0 {
				items = []interface{}{map[string]interface{}{"name": "a.txt", "isDir": false, "size": targetSize}}
			}
			envelope(w, map[string]interface{}{"items": items})
		case "/api/v2/files/upload":
			uploadCount++
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			uploadedDir = r.FormValue("path")
			f, hdr, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer f.Close()
			uploadedName = hdr.Filename
			uploaded, _ = io.ReadAll(f)
			envelope(w, map[string]interface{}{})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(remote.Close)
	target := insertTestNode(t, "node-b", remote.URL, "online")
	return NewIClusterSyncService(), target, &uploaded, &uploadedName, &uploadedDir, &uploadCount
}

func TestSyncFileTransferByteIdenticalOverwrite(t *testing.T) {
	content := []byte("sync-bytes\x00\x01\x02\xffhello-sync!")
	wantHash := sha256.Sum256(content)
	svc, target, uploaded, uploadedName, uploadedDir, uploadCount := fileTransferFixture(t, content, 999)
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "file", SourceNodeID: 0, TargetNodeIDs: []uint{target.ID},
		ResourceKeys: []string{"/data/a.txt"}, Path: "/data", OverwriteConflicts: true,
	})
	if err != nil {
		t.Fatalf("Execute local->remote file: %v", err)
	}
	if *uploadCount != 1 {
		t.Fatalf("upload calls = %d, want 1", *uploadCount)
	}
	if *uploadedName != "a.txt" || *uploadedDir != "/data" {
		t.Errorf("upload = name %q dir %q, want a.txt /data", *uploadedName, *uploadedDir)
	}
	if gotHash := sha256.Sum256(*uploaded); gotHash != wantHash {
		t.Errorf("uploaded sha256 = %x, want %x (bytes differ from source)", gotHash, wantHash)
	}
	records, err := svc.TaskRecords(taskID)
	if err != nil {
		t.Fatalf("TaskRecords: %v", err)
	}
	if len(records) != 1 || records[0].Action != model.ClusterSyncActionCreate {
		t.Errorf("records = %+v, want one create record", records)
	}
}

func TestSyncFileTransferSkipConflict(t *testing.T) {
	content := []byte("sync-bytes\x00\x01\x02\xffhello-sync!")
	svc, target, _, _, _, uploadCount := fileTransferFixture(t, content, 999)
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "file", SourceNodeID: 0, TargetNodeIDs: []uint{target.ID},
		ResourceKeys: []string{"/data/a.txt"}, Path: "/data", OverwriteConflicts: false,
	})
	if err != nil {
		t.Fatalf("Execute local->remote file: %v", err)
	}
	if *uploadCount != 0 {
		t.Errorf("upload calls = %d, want 0 (conflict must be skipped, not uploaded)", *uploadCount)
	}
	records, err := svc.TaskRecords(taskID)
	if err != nil {
		t.Fatalf("TaskRecords: %v", err)
	}
	if len(records) != 1 || records[0].Action != model.ClusterSyncActionSkipConflict {
		t.Errorf("records = %+v, want one skip-conflict record", records)
	}
}

func TestSyncFileTransferByteIdenticalRemoteToLocal(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	content := []byte("\x00\x01\x02\xffremote-bytes-1234567890")
	wantHash := sha256.Sum256(content)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []map[string]interface{}{
				{"name": "b.bin", "isDir": false, "size": len(content)},
			}})
		case "/api/v2/files/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	source := insertTestNode(t, "node-b", remote.URL, "online")
	var uploaded []byte
	var uploadedName string
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []interface{}{}})
		case "/api/v2/files/upload":
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			f, hdr, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer f.Close()
			uploadedName = hdr.Filename
			uploaded, _ = io.ReadAll(f)
			envelope(w, map[string]interface{}{})
		default:
			http.NotFound(w, r)
		}
	}))
	svc := NewIClusterSyncService()
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "file", SourceNodeID: source.ID, TargetNodeIDs: []uint{0},
		ResourceKeys: []string{"/data/b.bin"}, Path: "/data",
	})
	if err != nil {
		t.Fatalf("Execute remote->local file: %v", err)
	}
	if uploadedName != "b.bin" {
		t.Errorf("uploaded name = %q, want b.bin", uploadedName)
	}
	if gotHash := sha256.Sum256(uploaded); gotHash != wantHash {
		t.Errorf("uploaded sha256 = %x, want %x (bytes differ from source)", gotHash, wantHash)
	}
	records, err := svc.TaskRecords(taskID)
	if err != nil {
		t.Fatalf("TaskRecords: %v", err)
	}
	if len(records) != 1 || records[0].Action != model.ClusterSyncActionCreate {
		t.Errorf("records = %+v, want one create record", records)
	}
}

func TestSyncRemoteDownloadJsonErrorRejected(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	// The agent answers errors as a JSON envelope with HTTP 200 (bad API
	// key, sensitive path, missing file). The download must reject that
	// body instead of saving it as the file and "syncing" corrupt bytes.
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []map[string]interface{}{
				{"name": "c.txt", "isDir": false, "size": 3},
			}})
		case "/api/v2/files/download":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 401, "message": "ErrApiConfigKeyInvalid", "data": nil})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	source := insertTestNode(t, "node-b", remote.URL, "online")
	uploadCount := 0
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []interface{}{}})
		case "/api/v2/files/upload":
			uploadCount++
			envelope(w, map[string]interface{}{})
		default:
			http.NotFound(w, r)
		}
	}))
	svc := NewIClusterSyncService()
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "file", SourceNodeID: source.ID, TargetNodeIDs: []uint{0},
		ResourceKeys: []string{"/data/c.txt"}, Path: "/data",
	})
	if err != nil {
		t.Fatalf("Execute remote->local file: %v", err)
	}
	if uploadCount != 0 {
		t.Errorf("upload calls = %d, want 0 (failed download must not upload)", uploadCount)
	}
	records, err := svc.TaskRecords(taskID)
	if err != nil {
		t.Fatalf("TaskRecords: %v", err)
	}
	if len(records) != 1 || records[0].Action != model.ClusterSyncActionFailed {
		t.Fatalf("records = %+v, want one failed record", records)
	}
	if !strings.Contains(records[0].Reason, "download from source failed") {
		t.Errorf("record reason = %q, want it to mention the failed download", records[0].Reason)
	}
}

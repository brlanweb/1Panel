package service

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
		ResourceKeys:  []string{"sha256:aaa", "sha256:bbb", "sha256:ccc"},
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("Preview returned %d items, want 3", len(items))
	}
	// Keys are image ids; a source tag owned by another image on the
	// target is a conflict.
	want := map[string]string{
		"sha256:aaa": model.ClusterSyncActionSkipSame,
		"sha256:bbb": model.ClusterSyncActionSkipConflict,
		"sha256:ccc": model.ClusterSyncActionCreate,
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
		dto.ClusterSyncResource{Key: "9", Name: "WordPress", Detail: "exif,igbinary,imagick"}, syncTransferOpts{})
	if action != model.ClusterSyncActionSkipSame {
		t.Errorf("transferOne same php template = %s, want skip-same", action)
	}
	// Different extensions, no overwrite -> skip-conflict.
	action, _ = svc.transferOne(dto.ClusterSyncKindPhp, sourceNode, targetNode,
		dto.ClusterSyncResource{Key: "9", Name: "WordPress", Detail: "gd,zip"}, syncTransferOpts{})
	if action != model.ClusterSyncActionSkipConflict {
		t.Errorf("transferOne conflicting php template = %s, want skip-conflict", action)
	}
	// New template -> created.
	action, _ = svc.transferOne(dto.ClusterSyncKindPhp, sourceNode, targetNode,
		dto.ClusterSyncResource{Key: "10", Name: "SeaCMS", Detail: "mysqli,pdo_mysql,gd,curl"}, syncTransferOpts{})
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
	if len(res) != 1 || res[0].Key != "sha256:aaa" || res[0].Name != "redis:7.2.5" || res[0].Detail != "redis:7.2.5" {
		t.Fatalf("resources = %+v, want one image sha256:aaa tagged redis:7.2.5", res)
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

func TestSyncFileTransferToTargetPath(t *testing.T) {
	content := []byte("to-another-dir")
	svc, target, uploaded, uploadedName, uploadedDir, _ := fileTransferFixture(t, content, -1)
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "file", SourceNodeID: 0, TargetNodeIDs: []uint{target.ID},
		ResourceKeys: []string{"/data/a.txt"}, Path: "/data", TargetPath: "/opt/backup",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if *uploadedName != "a.txt" || *uploadedDir != "/opt/backup" || string(*uploaded) != "to-another-dir" {
		t.Errorf("upload = name %q dir %q bytes %q, want a.txt into /opt/backup", *uploadedName, *uploadedDir, string(*uploaded))
	}
	records, _ := svc.TaskRecords(taskID)
	if len(records) != 1 || records[0].Action != model.ClusterSyncActionCreate {
		t.Errorf("records = %+v, want one create record", records)
	}
}

// fakeTaskLog answers /logs/tasks/search with Success for every task id it
// has seen created, so waitNodeTask returns immediately.
type fakeTaskLog struct{ seen map[string]bool }

func (f *fakeTaskLog) note(body map[string]interface{}) {
	if id, _ := body["taskID"].(string); id != "" {
		f.seen[id] = true
	}
}

func (f *fakeTaskLog) serve(w http.ResponseWriter, body map[string]interface{}) {
	id, _ := body["taskID"].(string)
	status := "Executing"
	if f.seen[id] {
		status = "Success"
	}
	envelope(w, map[string]interface{}{"total": 1, "items": []map[string]interface{}{{"id": id, "status": status}}})
}

func TestSyncDirectoryTransferUsesCompressAndDecompress(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	oldPoll := syncTaskPollInterval
	syncTaskPollInterval = time.Millisecond
	defer func() { syncTaskPollInterval = oldPoll }()

	srcLog := &fakeTaskLog{seen: map[string]bool{}}
	var compressBody map[string]interface{}
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []map[string]interface{}{{"name": "site", "isDir": true}}})
		case "/api/v2/files":
			envelope(w, nil)
		case "/api/v2/files/compress":
			compressBody = body
			srcLog.note(body)
			envelope(w, nil)
		case "/api/v2/logs/tasks/search":
			srcLog.serve(w, body)
		case "/api/v2/files/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("tar-bytes"))
		case "/api/v2/files/del":
			envelope(w, nil)
		default:
			http.NotFound(w, r)
		}
	}))
	dstLog := &fakeTaskLog{seen: map[string]bool{}}
	var uploadDir string
	var decompressBody map[string]interface{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if r.Header.Get("Content-Type") == "application/json" {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch r.URL.Path {
		case "/api/v2/files/search":
			envelope(w, map[string]interface{}{"items": []interface{}{}})
		case "/api/v2/files/upload":
			_ = r.ParseMultipartForm(32 << 20)
			uploadDir = r.FormValue("path")
			envelope(w, nil)
		case "/api/v2/files/decompress":
			decompressBody = body
			dstLog.note(body)
			envelope(w, nil)
		case "/api/v2/logs/tasks/search":
			dstLog.serve(w, body)
		case "/api/v2/files/del":
			envelope(w, nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	target := insertTestNode(t, "node-b", remote.URL, "online")

	svc := NewIClusterSyncService()
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "file", SourceNodeID: 0, TargetNodeIDs: []uint{target.ID},
		ResourceKeys: []string{"/www/site"}, Path: "/www", TargetPath: "/srv",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	records, _ := svc.TaskRecords(taskID)
	if len(records) != 1 || records[0].Action != model.ClusterSyncActionCreate {
		t.Fatalf("records = %+v, want one create record", records)
	}
	if files, _ := compressBody["files"].([]interface{}); len(files) != 1 || files[0] != "/www/site" || compressBody["type"] != "tar.gz" {
		t.Errorf("compress body = %v, want /www/site as tar.gz", compressBody)
	}
	if uploadDir != syncTmpDir {
		t.Errorf("archive uploaded into %q, want %q", uploadDir, syncTmpDir)
	}
	if decompressBody["dst"] != "/srv" || decompressBody["overwrite"] != false {
		t.Errorf("decompress body = %v, want dst /srv overwrite false", decompressBody)
	}
}

func TestSyncAppDatabasesIntersection(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	serve := func(list []map[string]interface{}) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/v2/databases/db/list/") {
				http.NotFound(w, r)
				return
			}
			envelope(w, list)
		}))
	}
	a := serve([]map[string]interface{}{{"type": "mysql", "database": "mysql"}, {"type": "postgresql", "database": "pg"}})
	defer a.Close()
	b := serve([]map[string]interface{}{{"type": "mysql", "database": "mysql"}, {"type": "mariadb", "database": "maria"}})
	defer b.Close()
	na := insertTestNode(t, "a", a.URL, "online")
	nb := insertTestNode(t, "b", b.URL, "online")
	list, err := NewIClusterSyncService().AppDatabases(dto.ClusterSyncAppDBReq{NodeIDs: []uint{na.ID, nb.ID}})
	if err != nil {
		t.Fatalf("AppDatabases: %v", err)
	}
	if len(list) != 1 || list[0].Type != "mysql" || list[0].Database != "mysql" {
		t.Fatalf("databases = %+v, want only mysql/mysql", list)
	}
}

func TestSyncAppUnsupportedReason(t *testing.T) {
	cases := []struct {
		res  dto.ClusterSyncResource
		want bool
	}{
		{dto.ClusterSyncResource{Detail: "openresty 1.27"}, true},
		{dto.ClusterSyncResource{Detail: "wordpress 6.5", Meta: map[string]interface{}{"dbType": "mysql"}}, false},
		{dto.ClusterSyncResource{Detail: "halo 2.0", Meta: map[string]interface{}{"dbType": "postgresql"}}, false},
		{dto.ClusterSyncResource{Detail: "x 1", Meta: map[string]interface{}{"hasRedis": true}}, true},
		{dto.ClusterSyncResource{Detail: "y 1", Meta: map[string]interface{}{"dbType": "mongodb"}}, true},
		{dto.ClusterSyncResource{Detail: "z 1"}, false},
	}
	for _, c := range cases {
		if got := appUnsupportedReason(c.res) != ""; got != c.want {
			t.Errorf("appUnsupportedReason(%+v) unsupported = %v, want %v", c.res, got, c.want)
		}
	}
}

// fakeImageNode emulates the agent endpoints used by an image transfer:
// file search / mkdir, image save (writes <path>/<name>.tar as a task),
// download, upload, image load (as a task), image search and task search.
type fakeImageNode struct {
	files  map[string][]byte
	images map[string][]string // id -> tags
	tasks  map[string]string   // taskID -> status
	saved  string
	loaded string
}

func newFakeImageNode() *fakeImageNode {
	return &fakeImageNode{files: map[string][]byte{}, images: map[string][]string{}, tasks: map[string]string{}}
}

func (n *fakeImageNode) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch r.URL.Path {
		case "/api/v2/files/search":
			dir, _ := body["path"].(string)
			items := []map[string]interface{}{}
			if dir == "/tmp" {
				items = append(items, map[string]interface{}{"name": "1panel-cluster-sync", "isDir": true})
			}
			envelope(w, map[string]interface{}{"items": items, "itemTotal": len(items)})
		case "/api/v2/files":
			envelope(w, nil)
		case "/api/v2/containers/image/save":
			id, _ := body["taskID"].(string)
			p := body["path"].(string) + "/" + body["name"].(string) + ".tar"
			n.files[p] = []byte("image-tar:" + body["tagName"].(string))
			n.saved = body["tagName"].(string)
			n.tasks[id] = "Success"
			envelope(w, nil)
		case "/api/v2/files/download":
			data, ok := n.files[r.URL.Query().Get("path")]
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 400, "message": "ErrFileNotFound"})
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(data)
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
			raw, _ := io.ReadAll(f)
			_ = f.Close()
			n.files[r.FormValue("path")+"/"+hdr.Filename] = raw
			envelope(w, nil)
		case "/api/v2/containers/image/load":
			id, _ := body["taskID"].(string)
			paths, _ := body["paths"].([]interface{})
			if len(paths) == 1 {
				if data, ok := n.files[paths[0].(string)]; ok {
					// docker load accepts gzip-compressed tarballs as-is.
					if zr, err := gzip.NewReader(bytes.NewReader(data)); err == nil {
						if plain, err := io.ReadAll(zr); err == nil {
							data = plain
						}
					}
					n.loaded = string(data)
					n.tasks[id] = "Success"
				} else {
					n.tasks[id] = "Failed"
				}
			}
			envelope(w, nil)
		case "/api/v2/containers/image/search":
			items := []map[string]interface{}{}
			for id, tags := range n.images {
				items = append(items, map[string]interface{}{"id": id, "tags": tags, "size": 10})
			}
			envelope(w, map[string]interface{}{"total": len(items), "items": items})
		case "/api/v2/logs/tasks/search":
			id, _ := body["taskID"].(string)
			items := []map[string]interface{}{}
			if st, ok := n.tasks[id]; ok {
				items = append(items, map[string]interface{}{"id": id, "status": st})
			}
			envelope(w, map[string]interface{}{"total": len(items), "items": items})
		case "/api/v2/files/del":
			envelope(w, nil)
		case "/api/v2/files/chunkupload":
			// Older agent without resumable upload: master falls back.
			http.NotFound(w, r)
		default:
			t.Logf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

func TestSyncImageTransferSaveMoveLoad(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	oldPoll := syncTaskPollInterval
	syncTaskPollInterval = time.Millisecond
	defer func() { syncTaskPollInterval = oldPoll }()

	src := newFakeImageNode()
	src.images["sha256:redis"] = []string{"redis:7.2.5"}
	startLocalAgentTestServer(t, src.handler(t))
	dst := newFakeImageNode()
	remote := httptest.NewServer(dst.handler(t))
	defer remote.Close()
	target := insertTestNode(t, "uk01", remote.URL, "online")

	svc := NewIClusterSyncService()
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "image", SourceNodeID: 0, TargetNodeIDs: []uint{target.ID}, ResourceKeys: []string{"sha256:redis"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	records, _ := svc.TaskRecords(taskID)
	if len(records) != 1 || records[0].Action != model.ClusterSyncActionCreate {
		t.Fatalf("records = %+v, want one create record", records)
	}
	if src.saved != "redis:7.2.5" || dst.loaded != "image-tar:redis:7.2.5" {
		t.Errorf("saved %q loaded %q, want the source tarball loaded on the target", src.saved, dst.loaded)
	}
	var task model.ClusterSyncTask
	_ = global.DB.First(&task, taskID).Error
	if task.Status != model.ClusterSyncTaskSuccess || task.Step != "" {
		t.Errorf("task status %q step %q, want success and cleared step", task.Status, task.Step)
	}
}

func TestSyncWaitNodeTaskFailsWhenTaskMissing(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	oldPoll := syncTaskPollInterval
	oldLimit := syncTaskMissingLimit
	syncTaskPollInterval = 10 * time.Millisecond
	syncTaskMissingLimit = 50 * time.Millisecond
	defer func() { syncTaskPollInterval = oldPoll; syncTaskMissingLimit = oldLimit }()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": 0, "items": []interface{}{}})
	}))
	defer remote.Close()
	node := insertTestNode(t, "n", remote.URL, "online")
	svc := NewIClusterSyncService().(*ClusterSyncService)
	start := time.Now()
	err := svc.waitNodeTask(node, "nope", 10*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("waitNodeTask = %v, want a not-found error", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("waitNodeTask took %s", time.Since(start))
	}
}

// Cancelling an image sync while the target upload is in flight ends the
// task as "cancelled" and removes the export archive on the source.
func TestSyncCancelImageTaskCleansUp(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	oldPoll := syncTaskPollInterval
	syncTaskPollInterval = time.Millisecond
	defer func() { syncTaskPollInterval = oldPoll }()

	src := newFakeImageNode()
	src.images["sha256:redis"] = []string{"redis:7.2.5"}
	var mu sync.Mutex
	var deleted []string
	srcHandler := src.handler(t)
	startLocalAgentTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/files/del" {
			var body map[string]interface{}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			mu.Lock()
			deleted = append(deleted, body["path"].(string))
			mu.Unlock()
			envelope(w, nil)
			return
		}
		srcHandler(w, r)
	}))
	// Target hangs on upload until the request is cancelled.
	uploadStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/files/chunkupload", "/api/v2/files/upload":
			select {
			case uploadStarted <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
				t.Logf("upload %s aborted by client (parallel=%q)", r.URL.Path, r.FormValue("parallel"))
			case <-release:
				t.Logf("upload %s still open at test end", r.URL.Path)
			}
		default:
			envelope(w, map[string]interface{}{"total": 0, "items": []interface{}{}})
		}
	}))
	defer remote.Close()
	defer close(release) // runs before remote.Close
	target := insertTestNode(t, "cd", remote.URL, "online")

	svc := NewIClusterSyncService().(*ClusterSyncService)
	syncExecuteAsync = true
	taskID, err := svc.Execute(dto.ClusterSyncExecuteReq{
		Kind: "image", SourceNodeID: 0, TargetNodeIDs: []uint{target.ID}, ResourceKeys: []string{"sha256:redis"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	select {
	case <-uploadStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("upload never started")
	}
	if err := svc.CancelTask(taskID); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var task model.ClusterSyncTask
	for time.Now().Before(deadline) {
		_ = global.DB.First(&task, taskID).Error
		if task.Status != model.ClusterSyncTaskRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if task.Status != model.ClusterSyncTaskCancelled {
		t.Fatalf("status = %q, want cancelled", task.Status)
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, p := range deleted {
		if strings.HasPrefix(p, syncTmpDir+"/sync-image-") && strings.HasSuffix(p, ".tar") {
			found = true
		}
	}
	if !found {
		t.Fatalf("source export archive not removed, deleted=%v", deleted)
	}
	records, _ := svc.TaskRecords(taskID)
	if len(records) != 1 || records[0].Action != model.ClusterSyncActionCancelled {
		t.Fatalf("records = %+v, want one cancelled record", records)
	}
}

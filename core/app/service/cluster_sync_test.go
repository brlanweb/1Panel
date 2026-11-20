package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	return func() {
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
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

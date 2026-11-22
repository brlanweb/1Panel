package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/global"
)

// fakeNodePanel answers the remote endpoints the overview and the node
// upgrade use, and records the upgrade request it received.
type fakeNodePanel struct {
	mu          sync.Mutex
	version     string
	upgradeBody map[string]string
	unreachable bool
}

func (f *fakeNodePanel) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.unreachable {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.Header.Get("1Panel-Token") == "" {
			t.Errorf("%s sent without API auth headers", r.URL.Path)
		}
		switch r.URL.Path {
		case "/api/v2/settings/search":
			envelope(w, map[string]string{"systemVersion": f.version})
		case "/api/v2/dashboard/current/all/all":
			envelope(w, map[string]interface{}{"diskData": []map[string]interface{}{
				{"path": "/data", "total": 2000, "used": 500, "usedPercent": 25.0},
				{"path": "/", "total": 1000, "used": 900, "usedPercent": 90.0},
				{"path": "/empty", "total": 0},
			}})
		case "/api/v2/core/settings/upgrade":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.upgradeBody = body
			envelope(w, nil)
		case "/api/v2/core/settings/upgrade/status":
			envelope(w, dto.UpgradeStatus{Stage: "downloading", Version: "v2.5.1-cluster.5", CurrentVersion: f.version, Done: 10, Total: 100})
		default:
			t.Errorf("unexpected remote call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func releaseTags(tags ...string) []forkRelease {
	list := make([]forkRelease, 0, len(tags))
	for _, tag := range tags {
		list = append(list, forkRelease{TagName: tag})
	}
	return list
}

func setupUpgradeTestDB(t *testing.T) func() {
	restore := setupSyncTestDB(t)
	if err := global.DB.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	return restore
}

func TestUpgradeNodeForwardsLatestRelease(t *testing.T) {
	defer setupUpgradeTestDB(t)()
	withFakeReleases(t, releaseTags("v2.5.1-cluster.4", "v2.5.1-cluster.5"))
	fake := &fakeNodePanel{version: "v2.5.1-cluster.3"}
	remote := httptest.NewServer(fake.handler(t))
	defer remote.Close()
	node := insertTestNode(t, "node-up", remote.URL, model.ClusterNodeStatusOnline)

	svc := &ClusterNodeService{}
	if err := svc.UpgradeNode(dto.ClusterNodeUpgradeReq{ID: node.ID}); err != nil {
		t.Fatalf("UpgradeNode: %v", err)
	}
	if got := fake.upgradeBody["version"]; got != "v2.5.1-cluster.5" {
		t.Fatalf("remote upgrade version = %q, want the newest release v2.5.1-cluster.5", got)
	}
}

func TestUpgradeNodeRejectsNotNewer(t *testing.T) {
	defer setupUpgradeTestDB(t)()
	withFakeReleases(t, releaseTags("v2.5.1-cluster.5"))
	fake := &fakeNodePanel{version: "v2.5.1-cluster.5"}
	remote := httptest.NewServer(fake.handler(t))
	defer remote.Close()
	node := insertTestNode(t, "node-latest", remote.URL, model.ClusterNodeStatusOnline)

	err := (&ClusterNodeService{}).UpgradeNode(dto.ClusterNodeUpgradeReq{ID: node.ID})
	if err == nil || !strings.Contains(err.Error(), "最新版本") {
		t.Fatalf("UpgradeNode err = %v, want already-latest error", err)
	}
	if fake.upgradeBody != nil {
		t.Fatal("upgrade must not be forwarded when the node is already up to date")
	}
}

func TestNodeUpgradeStatusUnreachableWhileRestarting(t *testing.T) {
	defer setupUpgradeTestDB(t)()
	fake := &fakeNodePanel{version: "v2.5.1-cluster.3"}
	remote := httptest.NewServer(fake.handler(t))
	defer remote.Close()
	node := insertTestNode(t, "node-status", remote.URL, model.ClusterNodeStatusOnline)
	svc := &ClusterNodeService{}

	status, err := svc.NodeUpgradeStatus(node.ID)
	if err != nil || !status.Reachable || status.Stage != "downloading" || status.Total != 100 {
		t.Fatalf("status = %+v err=%v, want reachable downloading 10/100", status, err)
	}
	stored, _ := clusterNodeRepo.Get(repo.WithByID(node.ID))
	if stored.Version != "v2.5.1-cluster.3" {
		t.Fatalf("stored version = %q, want it refreshed from the status", stored.Version)
	}

	fake.mu.Lock()
	fake.unreachable = true
	fake.mu.Unlock()
	status, err = svc.NodeUpgradeStatus(node.ID)
	if err != nil {
		t.Fatalf("an unreachable (restarting) node must not be an error: %v", err)
	}
	if status.Reachable {
		t.Fatal("status.Reachable = true for an unreachable node")
	}
}

func TestFillNodeExtrasVersionAndRootDisk(t *testing.T) {
	defer setupUpgradeTestDB(t)()
	fake := &fakeNodePanel{version: "v2.5.1-cluster.4"}
	remote := httptest.NewServer(fake.handler(t))
	defer remote.Close()
	node := insertTestNode(t, "node-extras", remote.URL, model.ClusterNodeStatusOnline)

	var snap dto.ClusterNodeSnapshot
	upMap := (&ClusterNodeService{}).fillNodeExtras(node, &snap)
	if snap.Version != "v2.5.1-cluster.4" || upMap["version"] != "v2.5.1-cluster.4" {
		t.Fatalf("version = %q / %v, want v2.5.1-cluster.4", snap.Version, upMap["version"])
	}
	if len(snap.Disks) != 2 || snap.Disks[0].Path != "/" {
		t.Fatalf("disks = %+v, want 2 disks with / first (zero-size mount dropped)", snap.Disks)
	}
	if snap.DiskUsedPercent != 90 || snap.DiskTotal != 1000 || snap.DiskUsed != 900 {
		t.Fatalf("summary disk = %v%% %d/%d, want the root mount 90%% 900/1000", snap.DiskUsedPercent, snap.DiskUsed, snap.DiskTotal)
	}
}

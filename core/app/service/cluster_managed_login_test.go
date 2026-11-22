package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/auth"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/global"
)

func setupManagedLoginDB(t *testing.T) func() {
	t.Helper()
	restore := setupSyncTestDB(t)
	if err := global.DB.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	return restore
}

func TestManagedLoginStateTransitions(t *testing.T) {
	defer setupManagedLoginDB(t)()

	if auth.IsManagedLoginLocked() {
		t.Fatal("fresh panel must not be locked")
	}
	state, err := auth.UpdateManagedLogin(true, 0, "master (1.2.3.4)")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Locked || !auth.IsManagedLoginLocked() || state.Master != "master (1.2.3.4)" {
		t.Fatalf("lock not applied: %+v", state)
	}

	// Temporary window: unlocked now, but still "enabled" so it relocks.
	state, _ = auth.UpdateManagedLogin(false, 30, "")
	if state.Locked || !state.Enabled || state.UnlockUntil <= time.Now().Unix() {
		t.Fatalf("temporary open wrong: %+v", state)
	}
	// Simulate the window expiring.
	_ = repo.NewISettingRepo().UpdateOrCreate(auth.SettingManagedLoginUnlockUntil, "1")
	if !auth.IsManagedLoginLocked() {
		t.Fatal("login must relock after the temporary window expires")
	}

	state, _ = auth.UpdateManagedLogin(false, 0, "")
	if state.Locked || state.Enabled || auth.IsManagedLoginLocked() {
		t.Fatalf("permanent unlock wrong: %+v", state)
	}
	if state.Master != "master (1.2.3.4)" {
		t.Fatalf("master name must be kept when not provided, got %q", state.Master)
	}
}

func TestSetNodesLoginLockReportsPerNode(t *testing.T) {
	defer setupManagedLoginDB(t)()

	var gotBody dto.ClusterManagedLoginReq
	modern := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != managedLoginPath {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("1Panel-Token") == "" {
			t.Error("managed login call without API auth headers")
		}
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		envelope(w, auth.ManagedLoginState{Supported: true, Locked: gotBody.Locked, Enabled: gotBody.Locked})
	}))
	defer modern.Close()
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer legacy.Close()

	n1 := insertTestNode(t, "modern", modern.URL, "online")
	n2 := insertTestNode(t, "legacy", legacy.URL, "online")

	svc := &ClusterNodeService{lastTest: map[uint]time.Time{}}
	res, err := svc.SetNodesLoginLock(dto.ClusterNodeLoginLockReq{NodeIDs: []uint{n1.ID, n2.ID, 9999}, Locked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("want 3 results, got %d", len(res))
	}
	if !res[0].Success || !res[0].Locked || !gotBody.Locked {
		t.Fatalf("modern node result wrong: %+v body=%+v", res[0], gotBody)
	}
	if res[1].Success || res[1].Supported {
		t.Fatalf("legacy node must fail as unsupported: %+v", res[1])
	}
	if res[2].Success {
		t.Fatalf("missing node must fail: %+v", res[2])
	}
	stored, _ := clusterNodeRepo.Get(repo.WithByID(n1.ID))
	if stored.LoginLocked == nil || !*stored.LoginLocked {
		t.Fatalf("lock snapshot not persisted: %+v", stored.LoginLocked)
	}
}

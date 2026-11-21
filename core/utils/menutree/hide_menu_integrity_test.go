package menutree

import (
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
)

func TestReconcileRemovesXpackMenu(t *testing.T) {
	menus := []dto.ShowMenu{
		{ID: "1", Label: "Home-Menu", Path: "/"},
		{ID: "11", Label: "Xpack-Menu", Children: []dto.ShowMenu{
			{ID: "118", Label: "XApp", Path: "/xpack/app"},
			{ID: "112", Label: "Dashboard", Path: "/xpack/waf/dashboard"},
		}},
		{ID: "124", Label: "Cluster-Menu", Path: "/cluster", Children: []dto.ShowMenu{
			{ID: "1241", Label: "ClusterOverview", Path: "/cluster/overview"},
		}},
	}
	got, changed := ReconcileHideMenuIntegrity(menus, nil)
	if !changed {
		t.Fatal("expected a change")
	}
	if len(got) != 2 || got[0].Label != "Home-Menu" || got[1].Label != "Cluster-Menu" {
		t.Fatalf("unexpected menus: %+v", got)
	}
	if len(got[1].Children) != 1 {
		t.Fatalf("cluster children must be kept: %+v", got[1].Children)
	}
	again, changed := ReconcileHideMenuIntegrity(got, nil)
	if changed || len(again) != 2 {
		t.Fatalf("reconcile must be idempotent: %+v", again)
	}
}

func TestReconcileRemovesStrayXApp(t *testing.T) {
	menus := []dto.ShowMenu{
		{ID: "9", Label: "Toolbox-Menu", Children: []dto.ShowMenu{{ID: "118", Label: "XApp", Path: "/xpack/app"}}},
	}
	got, changed := ReconcileHideMenuIntegrity(menus, nil)
	if !changed || len(got) != 1 || len(got[0].Children) != 0 {
		t.Fatalf("XApp must be removed everywhere: %+v", got)
	}
}

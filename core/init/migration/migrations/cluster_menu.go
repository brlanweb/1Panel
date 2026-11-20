package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/init/migration/helper"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// clusterMenuChildren is the full sidebar subtree for Cluster Management,
// in display order. Labels must match the frontend route names in
// frontend/src/routers/modules/cluster.ts exactly: the sidebar is built by
// matching HideMenu labels against route names, and any route missing from
// HideMenu is dropped from the sidebar.
var clusterMenuChildren = []dto.ShowMenu{
	{ID: "1241", Disabled: false, Title: "menu.clusterOverview", IsShow: true, Label: "ClusterOverview", Path: "/cluster/overview", Sort: 10},
	{ID: "1242", Disabled: false, Title: "menu.clusterNodes", IsShow: true, Label: "ClusterNodes", Path: "/cluster/nodes", Sort: 20},
	{ID: "1243", Disabled: false, Title: "menu.clusterPanels", IsShow: true, Label: "ClusterPanels", Path: "/cluster/panels", Sort: 30},
	{ID: "1244", Disabled: false, Title: "menu.clusterWaf", IsShow: true, Label: "ClusterWaf", Path: "/cluster/waf", Sort: 40},
	{ID: "1245", Disabled: false, Title: "menu.clusterSync", IsShow: true, Label: "ClusterSync", Path: "/cluster/sync", Sort: 50},
	{ID: "1246", Disabled: false, Title: "menu.clusterMonitor", IsShow: true, Label: "ClusterMonitor", Path: "/cluster/monitor", Sort: 60},
}

// ExpandClusterMenu fixes the sidebar entry for Cluster Management.
//
// Background: AddClusterNodesMenu (20261119) seeded only a single flat
// top-level item (Label "ClusterNodes", Path /cluster/nodes, no children).
// The sidebar builder (frontend/src/utils/menu.ts) walks the HideMenu tree
// and drops every route that is not in it, so the overview / panels / WAF /
// sync / monitor pages existed as routes but had no sidebar entry at all —
// the monitor page was unreachable from the UI and looked "not built".
//
// This migration expands that leaf into a parent (Label "Cluster-Menu",
// Path /cluster, the frontend route name) with the six children above. It
// runs on already-deployed instances too: gormigrate applies any migration
// whose ID has not run yet, so adding a new migration (rather than editing
// the old one) is what makes the fix take effect on existing installs.
// Idempotent: re-running keeps any IsShow choice the user already made for
// a child, and never duplicates children.
var ExpandClusterMenu = &gormigrate.Migration{
	ID: "20261120-expand-cluster-menu",
	Migrate: func(tx *gorm.DB) error {
		return helper.UpdateHideMenu(tx, func(menus []dto.ShowMenu) []dto.ShowMenu {
			for i := range menus {
				if menus[i].Label != "ClusterNodes" && menus[i].Label != "Cluster-Menu" {
					continue
				}
				// Preserve the user's show/hide choice for the parent.
				parentShow := menus[i].IsShow
				// Preserve per-child show/hide choices from a previous run.
				kept := map[string]bool{}
				for _, child := range menus[i].Children {
					kept[child.Label] = child.IsShow
				}
				children := make([]dto.ShowMenu, 0, len(clusterMenuChildren))
				for _, child := range clusterMenuChildren {
					if show, ok := kept[child.Label]; ok {
						child.IsShow = show
					}
					children = append(children, child)
				}
				menus[i].ID = "124"
				menus[i].Label = "Cluster-Menu"
				menus[i].Title = "menu.cluster"
				menus[i].Path = "/cluster"
				menus[i].Sort = 1050
				menus[i].IsShow = parentShow
				menus[i].Children = children
				return menus
			}
			// No cluster entry at all (fresh DB where the old migration did
			// not run, or the user deleted it): append the full subtree.
			return append(menus, dto.ShowMenu{
				ID: "124", Disabled: false, Title: "menu.cluster", IsShow: true,
				Label: "Cluster-Menu", Path: "/cluster", Sort: 1050,
				Children: clusterMenuChildren,
			})
		})
	},
}

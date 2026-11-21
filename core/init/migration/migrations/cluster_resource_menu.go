package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/init/migration/helper"
	"github.com/1Panel-dev/1Panel/core/utils/menutree"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddClusterResourceMenu adds the cluster resource monitor page
// (/cluster/resource, route name ClusterResource) right after the overview.
// The label must equal the frontend route name, otherwise the sidebar
// builder drops the entry.
var AddClusterResourceMenu = &gormigrate.Migration{
	ID: "20261121-add-cluster-resource-menu",
	Migrate: func(tx *gorm.DB) error {
		return helper.UpsertChildMenuByLabel(tx, "Cluster-Menu", dto.ShowMenu{
			ID: "1248", Disabled: false, Title: "menu.clusterResource", IsShow: true,
			Label: "ClusterResource", Path: "/cluster/resource", Sort: 15,
		}, "ClusterOverview")
	},
}

// RemoveXpackMenuTree removes the "advanced features" (Xpack-Menu) entry and
// all of its children from existing installs. None of those paid pages exist
// in this fork, so the menu only led to empty routes.
var RemoveXpackMenuTree = &gormigrate.Migration{
	ID: "20261121-remove-xpack-menu-tree",
	Migrate: func(tx *gorm.DB) error {
		return helper.UpdateHideMenu(tx, func(menus []dto.ShowMenu) []dto.ShowMenu {
			updated, _ := menutree.ReconcileHideMenuIntegrity(menus, nil)
			return updated
		})
	},
}

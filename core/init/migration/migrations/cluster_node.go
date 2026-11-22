package migrations

import (
	"sort"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/init/migration/helper"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var AddClusterNodes = &gormigrate.Migration{
	ID:      "20261119-add-cluster-nodes",
	Migrate: func(tx *gorm.DB) error { return tx.AutoMigrate(&model.ClusterNode{}) },
}

// AddClusterNodesMenu adds a top-level "Cluster Management" menu (ID 124,
// unused) pointing at /cluster/nodes. It deliberately does NOT reuse the paid
// xpack cluster menu (ID 120, /xpack/cluster) and is not placed under the
// Xpack-Menu parent, so it is never mistaken for a paid feature.
// helper.AddMenu only inserts under a parent, so the top-level insert is done
// with UpdateHideMenu (same pattern as MoveVirtualMachineMenuToTopLevel);
// it is idempotent and preserves a user's IsShow choice on re-run.
var AddClusterNodesMenu = &gormigrate.Migration{
	ID: "20261119-add-cluster-nodes-menu",
	Migrate: func(tx *gorm.DB) error {
		return helper.UpdateHideMenu(tx, func(menus []dto.ShowMenu) []dto.ShowMenu {
			item := dto.ShowMenu{
				ID:       "124",
				Disabled: false,
				Title:    "menu.cluster",
				IsShow:   true,
				Label:    "ClusterNodes",
				Path:     "/cluster/nodes",
				Sort:     1050,
			}
			for _, menu := range menus {
				if menu.Label == item.Label && menu.Path == item.Path {
					return menus
				}
			}
			menus = append(menus, item)
			sort.SliceStable(menus, func(i, j int) bool { return menus[i].Sort < menus[j].Sort })
			return menus
		})
	},
}

// AddClusterNodeDiskColumns adds the disk usage snapshot columns.
var AddClusterNodeDiskColumns = &gormigrate.Migration{
	ID:      "20261122-add-cluster-node-disk",
	Migrate: func(tx *gorm.DB) error { return tx.AutoMigrate(&model.ClusterNode{}) },
}

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

// AddClusterNodeHardwareColumns adds the CPU temperature / GPU snapshot columns.
var AddClusterNodeHardwareColumns = &gormigrate.Migration{
	ID:      "20261124-add-cluster-node-hardware",
	Migrate: func(tx *gorm.DB) error { return tx.AutoMigrate(&model.ClusterNode{}) },
}

// AddClusterNodeDiskColumns adds the disk usage snapshot columns.
var AddClusterNodeDiskColumns = &gormigrate.Migration{
	ID:      "20261122-add-cluster-node-disk",
	Migrate: func(tx *gorm.DB) error { return tx.AutoMigrate(&model.ClusterNode{}) },
}

// AddClusterNodeAgentColumns adds mode / host_id / agent_port / proxy_id_cipher
// for agent-only (mTLS / reverse tunnel) nodes and SSH host enrollment.
var AddClusterNodeAgentColumns = &gormigrate.Migration{
	ID: "20261124-add-cluster-node-agent-columns",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.ClusterNode{}); err != nil {
			return err
		}
		return tx.Model(&model.ClusterNode{}).
			Where("mode IS NULL OR mode = ''").
			Update("mode", model.ClusterNodeModePanel).Error
	},
}

// AddSystemHostsMenu adds the "Host" entry under System-Menu reusing the
// terminal host list for SSH management and one-click cluster enrollment.
var AddSystemHostsMenu = &gormigrate.Migration{
	ID: "20261124-add-system-hosts-menu",
	Migrate: func(tx *gorm.DB) error {
		return helper.UpdateHideMenu(tx, func(menus []dto.ShowMenu) []dto.ShowMenu {
			item := dto.ShowMenu{
				ID:       "1009",
				Disabled: false,
				Title:    "terminal.host",
				IsShow:   true,
				Label:    "SystemHosts",
				Path:     "/hosts/servers",
				Sort:     5,
			}
			for i := range menus {
				if menus[i].Label != "System-Menu" {
					continue
				}
				for _, child := range menus[i].Children {
					if child.Label == item.Label || child.Path == item.Path {
						return menus
					}
				}
				menus[i].Children = append([]dto.ShowMenu{item}, menus[i].Children...)
				return menus
			}
			return menus
		})
	},
}

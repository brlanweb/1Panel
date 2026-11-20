package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/init/migration/helper"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddClusterDbReplicationTables creates the replication topology and
// failover-history tables. AutoMigrate is idempotent.
var AddClusterDbReplicationTables = &gormigrate.Migration{
	ID: "20261120-add-cluster-db-replication-tables",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.ClusterDbReplication{}); err != nil {
			return err
		}
		return tx.AutoMigrate(&model.ClusterDbFailover{})
	},
}

// AddClusterDbReplicationMenu appends the database-sync child (ID 1247)
// to the Cluster Management sidebar subtree on already-deployed
// instances. Fresh installs get it from clusterMenuChildren via
// ExpandClusterMenu; this migration covers instances where that already
// ran. Idempotent: the child is only appended when missing, and the
// user's show/hide choice for existing children is preserved.
var AddClusterDbReplicationMenu = &gormigrate.Migration{
	ID: "20261120-add-cluster-db-replication-menu",
	Migrate: func(tx *gorm.DB) error {
		return helper.UpdateHideMenu(tx, func(menus []dto.ShowMenu) []dto.ShowMenu {
			child := dto.ShowMenu{ID: "1247", Disabled: false, Title: "menu.clusterDbReplication", IsShow: true, Label: "ClusterDbReplication", Path: "/cluster/db-replication", Sort: 70}
			for i := range menus {
				if menus[i].Label != "ClusterNodes" && menus[i].Label != "Cluster-Menu" {
					continue
				}
				for _, existing := range menus[i].Children {
					if existing.Label == "ClusterDbReplication" {
						return menus
					}
				}
				menus[i].Children = append(menus[i].Children, child)
				return menus
			}
			return menus
		})
	},
}

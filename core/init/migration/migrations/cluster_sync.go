package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddClusterSyncTables creates the sync task/record tables. AutoMigrate is
// idempotent, so re-running this migration is safe.
var AddClusterSyncTables = &gormigrate.Migration{
	ID: "20261119-add-cluster-sync-tables",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.ClusterSyncTask{}); err != nil {
			return err
		}
		return tx.AutoMigrate(&model.ClusterSyncRecord{})
	},
}

// AddClusterNodeGroup creates the default "Default" group for cluster nodes
// (Type "cluster_node"). Group deletion switches on group.Type and only the
// xpack "node" type routes to MultiNodeProvider, so "cluster_node" never
// hits the paid path. The create is guarded so the migration is idempotent.
var AddClusterNodeGroup = &gormigrate.Migration{
	ID: "20261119-add-cluster-node-group",
	Migrate: func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Group{}).Where("type = ?", "cluster_node").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		return tx.Create(&model.Group{Name: "Default", Type: "cluster_node", IsDefault: true}).Error
	},
}

// AddClusterSyncTaskStep adds the "step" column (current transfer stage).
var AddClusterSyncTaskStep = &gormigrate.Migration{
	ID: "20261121-add-cluster-sync-task-step",
	Migrate: func(tx *gorm.DB) error {
		return tx.AutoMigrate(&model.ClusterSyncTask{})
	},
}

// AddClusterSyncTaskProgress adds byte progress columns (done/total/rate).
var AddClusterSyncTaskProgress = &gormigrate.Migration{
	ID: "20261121-add-cluster-sync-task-progress",
	Migrate: func(tx *gorm.DB) error {
		return tx.AutoMigrate(&model.ClusterSyncTask{})
	},
}

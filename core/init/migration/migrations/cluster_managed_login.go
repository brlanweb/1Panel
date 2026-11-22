package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddClusterManagedLogin adds the managed login lock settings (unlocked by
// default) and the lock snapshot columns of cluster nodes.
var AddClusterManagedLogin = &gormigrate.Migration{
	ID: "20261122-add-cluster-managed-login",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.ClusterNode{}); err != nil {
			return err
		}
		defaults := map[string]string{
			"ManagedLoginLock":        "Disable",
			"ManagedLoginUnlockUntil": "0",
			"ManagedLoginMaster":      "",
		}
		for key, value := range defaults {
			var count int64
			if err := tx.Model(&model.Setting{}).Where("key = ?", key).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := tx.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	},
}

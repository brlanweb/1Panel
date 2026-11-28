package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// ComputeUsageDaily: persisted daily usage of the model router (calls and
// tokens per day, model and caller), so the analytics totals survive
// restarts and the in-memory call log retention.
var ComputeUsageDaily = &gormigrate.Migration{
	ID: "20261127-compute-usage-daily",
	Migrate: func(tx *gorm.DB) error {
		return tx.AutoMigrate(&model.ComputeUsageDaily{})
	},
}

package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// ComputeInstanceUpstreamModel lets one pool span engines that serve the
// same model under different names (e.g. a w8a8 build on Ascend next to
// the bf16/fp8 build on GB10): compute_instances gains upstream_model, the
// name the engine itself expects. Existing rows keep it empty, which means
// "same as served_name", so nothing changes until an instance is edited.
var ComputeInstanceUpstreamModel = &gormigrate.Migration{
	ID: "20261128-compute-instance-upstream-model",
	Migrate: func(tx *gorm.DB) error {
		return tx.AutoMigrate(&model.ComputeInstance{})
	},
}

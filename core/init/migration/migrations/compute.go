package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/init/migration/helper"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddComputeTables creates the compute scheduling tables.
var AddComputeTables = &gormigrate.Migration{
	ID: "20261124-add-compute-tables",
	Migrate: func(tx *gorm.DB) error {
		return tx.AutoMigrate(&model.ComputeHost{}, &model.ComputeFabricLink{}, &model.ComputeRuntime{},
			&model.ComputeProfile{}, &model.ComputeInstance{}, &model.ComputeJob{}, &model.ComputeConsumer{},
			&model.ComputePolicy{})
	},
}

// AddComputeAcceleratorKind adds compute_runtimes.device (gpu / npu) and
// the built-in vLLM Ascend runtime is seeded at start-up.
var AddComputeAcceleratorKind = &gormigrate.Migration{
	ID: "20261125-add-compute-accelerator-kind",
	Migrate: func(tx *gorm.DB) error {
		return tx.AutoMigrate(&model.ComputeRuntime{})
	},
}

// AddComputeMenu added "算力调度" under the cluster menu (historical; it is
// moved under the AI menu by MoveComputeMenuToAI).
var AddComputeMenu = &gormigrate.Migration{
	ID: "20261124-add-compute-menu",
	Migrate: func(tx *gorm.DB) error {
		return helper.UpsertChildMenuByLabel(tx, "Cluster-Menu", dto.ShowMenu{
			ID: "1243", Disabled: false, Title: "menu.clusterCompute", IsShow: true,
			Label: "ClusterCompute", Path: "/cluster/compute", Sort: 16,
		}, "ClusterResource")
	},
}

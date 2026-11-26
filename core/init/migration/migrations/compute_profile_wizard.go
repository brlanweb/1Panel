package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// ComputeProfileWizard: deployment plans pick a prepared model (repo id),
// may be pinned to hosts (head first) and may stack several single-host
// replicas on one host (max_per_host). Existing plans keep working: empty
// host_ids = any host, max_per_host = 1, image empty = runtime image.
var ComputeProfileWizard = &gormigrate.Migration{
	ID: "20261129-compute-profile-wizard",
	Migrate: func(tx *gorm.DB) error {
		return tx.AutoMigrate(&model.ComputeProfile{})
	},
}

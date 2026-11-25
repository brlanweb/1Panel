package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/init/migration/helper"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// ComputeIntoModelServices folds compute scheduling into the AI pages:
//   - compute_policies gains the pool columns (name, account_id,
//     consumer_id) and the TTFT threshold;
//   - every model that has instances but no policy gets a pool with the
//     policy switched off, so nothing is lost when the old page goes away
//     (existing policies keep their mode);
//   - the stand-alone "算力调度" menu is removed: pools live in Model
//     Services, hosts in GPU, deployments and plans in vLLM.
//
// The model service of a pool is created on first use from the pool page
// ("bind"): the agent API is not reachable while core migrates.
var ComputeIntoModelServices = &gormigrate.Migration{
	ID: "20261127-compute-into-model-services",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.ComputePolicy{}); err != nil {
			return err
		}
		if err := tx.Model(&model.ComputePolicy{}).Where("name = '' OR name IS NULL").
			Update("name", gorm.Expr("served_name")).Error; err != nil {
			return err
		}
		var served []string
		if err := tx.Model(&model.ComputeInstance{}).Distinct("served_name").Pluck("served_name", &served).Error; err != nil {
			return err
		}
		for _, name := range served {
			if name == "" {
				continue
			}
			var n int64
			if err := tx.Model(&model.ComputePolicy{}).Where("served_name = ?", name).Count(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				continue
			}
			var profileID uint
			_ = tx.Model(&model.ComputeProfile{}).Where("served_name = ?", name).Order("id asc").Limit(1).Pluck("id", &profileID).Error
			p := model.ComputePolicy{Name: name, ServedName: name, ProfileID: profileID, Mode: "off", MinInstances: 0, MaxInstances: 1,
				Priority: 50, QueueHigh: 4, KVHigh: 0.92, SustainRounds: 3, IdleMinutes: 30, CooldownSec: 600}
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
		}
		return helper.UpdateHideMenu(tx, func(menus []dto.ShowMenu) []dto.ShowMenu {
			for i := range menus {
				kept := make([]dto.ShowMenu, 0, len(menus[i].Children))
				for _, child := range menus[i].Children {
					if child.Label == "ComputeScheduling" || child.Label == "ClusterCompute" {
						continue
					}
					kept = append(kept, child)
				}
				menus[i].Children = kept
			}
			return menus
		})
	},
}

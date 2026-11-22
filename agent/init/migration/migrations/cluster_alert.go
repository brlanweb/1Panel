package migrations

import (
	"encoding/json"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// clusterAlertTypes must be listed in the common config's resource send
// time range, otherwise CheckSendTimeRange drops every alert of the type.
var clusterAlertTypes = []string{"clusterResource", "siteProbe", "modelApi"}

// AddClusterAlertTargetState adds the per-target state table of the cluster
// alert types, registers the types in the send time range, and drops the
// license exception rules (license management was removed).
var AddClusterAlertTargetState = &gormigrate.Migration{
	ID: "20261122-add-cluster-alert-target-state",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.AlertTargetState{}); err != nil {
			return err
		}
		if err := tx.Where("type = ?", "licenseException").Delete(&model.Alert{}).Error; err != nil {
			return err
		}
		var config model.AlertConfig
		if err := tx.Where("type = ?", constant.CommonConfig).First(&config).Error; err != nil {
			// No common config yet: nothing to register.
			return nil
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(config.Config), &raw); err != nil {
			return nil
		}
		timeRange, _ := raw["alertSendTimeRange"].(map[string]interface{})
		if timeRange == nil {
			return nil
		}
		resource, _ := timeRange["resourceAlert"].(map[string]interface{})
		if resource == nil {
			return nil
		}
		types, _ := resource["type"].([]interface{})
		exists := map[string]bool{}
		kept := make([]interface{}, 0, len(types)+len(clusterAlertTypes))
		for _, item := range types {
			name, _ := item.(string)
			if name == "licenseException" {
				continue
			}
			exists[name] = true
			kept = append(kept, item)
		}
		for _, name := range clusterAlertTypes {
			if !exists[name] {
				kept = append(kept, name)
			}
		}
		resource["type"] = kept
		data, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		return tx.Model(&model.AlertConfig{}).Where("id = ?", config.ID).Update("config", string(data)).Error
	},
}

package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddForkReleaseSettings seeds the fork release channel settings used by
// the upgrade service (core/app/service/fork_release.go). Update checks,
// release notes and upgrade packages come from the fork's own GitHub
// releases, never from the upstream 1Panel package repository.
//
//   - ForkReleaseRepo: "owner/repo" whose releases are the update channel
//   - ForkReleaseAPI:  GitHub-compatible API base (change it for a Gitea /
//     GitHub Enterprise mirror)
//
// Existing values are kept, so re-pointing the channel survives upgrades.
var AddForkReleaseSettings = &gormigrate.Migration{
	ID: "20261121-add-fork-release-settings",
	Migrate: func(tx *gorm.DB) error {
		defaults := map[string]string{
			"ForkReleaseRepo": "brlanweb/1Panel",
			"ForkReleaseAPI":  "https://api.github.com",
		}
		for key, value := range defaults {
			var count int64
			if err := tx.Model(&model.Setting{}).Where("key = ?", key).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			if err := tx.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
				return err
			}
		}
		return nil
	},
}

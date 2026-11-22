package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// RenamePanelToXPanel switches the default panel name to XPanel. A name the
// user customised is left untouched.
var RenamePanelToXPanel = &gormigrate.Migration{
	ID: "20261122-rename-panel-xpanel",
	Migrate: func(tx *gorm.DB) error {
		return tx.Model(&model.Setting{}).
			Where("key = ? AND (value = ? OR value = ?)", "PanelName", "1Panel", "").
			Update("value", "XPanel").Error
	},
}

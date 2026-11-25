package migrations

import (
	"fmt"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/init/migration/helper"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// ComputeHostsReuseSources stops the compute module from keeping its own
// SSH credentials. Every existing compute host is re-pointed at a host
// that is managed elsewhere in the panel:
//
//   - linked to a cluster node            -> source node
//   - same addr/port/user in Terminal     -> source terminal (that entry)
//   - otherwise                           -> a Terminal → Hosts entry is
//     created with the old credentials (re-encrypted with the agent key)
//
// and the credential columns are dropped. Access keeps working, and the
// login is from now on edited in Terminal → Hosts only.
var ComputeHostsReuseSources = &gormigrate.Migration{
	ID: "20261126-compute-hosts-reuse-sources",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.ComputeHost{}, &model.ComputeRuntime{}, &model.ComputeInstance{}); err != nil {
			return err
		}
		m := tx.Migrator()
		if !m.HasColumn(&model.ComputeHost{}, "cred_cipher") {
			return nil
		}
		type oldHost struct {
			ID               uint
			Name             string
			Addr             string
			SSHPort          int    `gorm:"column:ssh_port"`
			SSHUser          string `gorm:"column:ssh_user"`
			AuthMode         string
			CredCipher       string
			PassPhraseCipher string
			ClusterNodeID    uint
		}
		var olds []oldHost
		if err := tx.Table("compute_hosts").Select("id, name, addr, ssh_port, ssh_user, auth_mode, cred_cipher, pass_phrase_cipher, cluster_node_id").Scan(&olds).Error; err != nil {
			return err
		}
		for _, h := range olds {
			source, refID, err := computeResolveSource(h.Name, h.Addr, h.SSHPort, h.SSHUser, h.AuthMode, h.CredCipher, h.PassPhraseCipher, h.ClusterNodeID)
			if err != nil {
				// Keep the host (topology / history stay) but mark it: the
				// operator re-imports it from Terminal → Hosts.
				global.LOG.Errorf("compute host %s: %v", h.Name, err)
				_ = tx.Table("compute_hosts").Where("id = ?", h.ID).Updates(map[string]interface{}{
					"source": model.ComputeSourceTerminal, "ref_id": 0, "status": model.ComputeHostOffline,
					"last_error": "migration: " + err.Error()}).Error
				continue
			}
			mode := model.ComputeModeAgentless
			if source != model.ComputeSourceTerminal {
				mode = model.ComputeModeAgent
			}
			if err := tx.Table("compute_hosts").Where("id = ?", h.ID).Updates(map[string]interface{}{
				"source": source, "ref_id": refID, "mode": mode}).Error; err != nil {
				return err
			}
		}
		for _, col := range []string{"cred_cipher", "pass_phrase_cipher", "ssh_port", "ssh_user", "auth_mode", "cluster_node_id"} {
			if m.HasColumn(&model.ComputeHost{}, col) {
				if err := m.DropColumn(&model.ComputeHost{}, col); err != nil {
					// Never leave credentials behind even if the column stays.
					_ = tx.Exec("UPDATE compute_hosts SET cred_cipher = '', pass_phrase_cipher = ''").Error
					global.LOG.Errorf("compute migration drop column %s: %v", col, err)
				}
			}
		}
		return nil
	},
}

type computeAgentHost struct {
	ID               uint `gorm:"primarykey"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	GroupID          uint
	Name             string
	Addr             string
	Port             int
	User             string
	AuthMode         string
	Password         string
	PrivateKey       string
	PassPhrase       string
	RememberPassword bool
	Description      string
}

func (computeAgentHost) TableName() string { return "hosts" }

func computeResolveSource(name, addr string, port int, user, authMode, credCipher, passCipher string, nodeID uint) (string, uint, error) {
	if nodeID > 0 {
		var n int64
		if err := global.DB.Model(&model.ClusterNode{}).Where("id = ?", nodeID).Count(&n).Error; err == nil && n > 0 {
			return model.ComputeSourceNode, nodeID, nil
		}
	}
	if global.AgentDB == nil {
		return "", 0, fmt.Errorf("agent database unavailable")
	}
	if port == 0 {
		port = 22
	}
	if user == "" {
		user = "root"
	}
	var exist computeAgentHost
	if err := global.AgentDB.Where("addr = ? AND port = ? AND user = ?", addr, port, user).First(&exist).Error; err == nil {
		return model.ComputeSourceTerminal, exist.ID, nil
	}
	if credCipher == "" {
		return "", 0, fmt.Errorf("no credential to move to Terminal → Hosts")
	}
	cred, err := encrypt.StringDecrypt(credCipher)
	if err != nil {
		return "", 0, fmt.Errorf("decrypt credential: %w", err)
	}
	pass := ""
	if passCipher != "" {
		if pass, err = encrypt.StringDecrypt(passCipher); err != nil {
			return "", 0, fmt.Errorf("decrypt passphrase: %w", err)
		}
	}
	var keySetting model.Setting
	if err := global.AgentDB.Where("key = ?", "EncryptKey").First(&keySetting).Error; err != nil {
		return "", 0, fmt.Errorf("read agent key: %w", err)
	}
	key := strings.TrimSpace(keySetting.Value)
	for len(key) < 16 {
		key += "u"
	}
	key = key[:16]
	enc := func(v string) (string, error) {
		if v == "" {
			return "", nil
		}
		return encrypt.StringEncryptWithKey(v, key)
	}
	host := computeAgentHost{Name: name, Addr: addr, Port: port, User: user, RememberPassword: true,
		Description: "imported from compute scheduling"}
	var group model.Group
	if err := global.AgentDB.Where("type = ? AND is_default = ?", "host", true).First(&group).Error; err == nil {
		host.GroupID = group.ID
	}
	if authMode == "key" {
		host.AuthMode = "key"
		if host.PrivateKey, err = enc(cred); err != nil {
			return "", 0, err
		}
		if host.PassPhrase, err = enc(pass); err != nil {
			return "", 0, err
		}
	} else {
		host.AuthMode = "password"
		if host.Password, err = enc(cred); err != nil {
			return "", 0, err
		}
	}
	if err := global.AgentDB.Create(&host).Error; err != nil {
		return "", 0, err
	}
	return model.ComputeSourceTerminal, host.ID, nil
}

// MoveComputeMenuToAI moves "算力调度" from the cluster menu to the AI
// menu, right after Model Services: it schedules the same models that
// AI → vLLM serves and reuses its deployments.
var MoveComputeMenuToAI = &gormigrate.Migration{
	ID: "20261126-move-compute-menu-to-ai",
	Migrate: func(tx *gorm.DB) error {
		show := true
		if err := helper.UpdateHideMenu(tx, func(menus []dto.ShowMenu) []dto.ShowMenu {
			for i := range menus {
				kept := make([]dto.ShowMenu, 0, len(menus[i].Children))
				for _, child := range menus[i].Children {
					if child.Label == "ClusterCompute" {
						show = child.IsShow
						continue
					}
					kept = append(kept, child)
				}
				menus[i].Children = kept
			}
			return menus
		}); err != nil {
			return err
		}
		return helper.UpsertChildMenuByLabel(tx, "AI-Menu", dto.ShowMenu{
			ID: "50", Disabled: false, Title: "menu.computeScheduling", IsShow: show,
			Label: "ComputeScheduling", Path: "/ai/compute", Sort: 80,
		}, "ModelServices")
	},
}

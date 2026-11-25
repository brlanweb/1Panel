package service

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
)

// Host sources of the compute module.
//
// The compute module never stores credentials. A compute host references
//   - a Terminal host (agent.db `hosts`): collected / operated over a pooled
//     SSH connection built from the credentials kept by the terminal page;
//   - a cluster node: reached through its signed panel API;
//   - this panel ("local"): reached over the agent unix socket.
// Panel hosts run scripts through the agent endpoint /api/v2/ai/compute/exec.

// computeTerminalHost mirrors agent.db `hosts` (read-only here).
type computeTerminalHost struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	UpdatedAt  time.Time `json:"updatedAt"`
	GroupID    uint      `json:"groupId"`
	Name       string    `json:"name"`
	Addr       string    `json:"addr"`
	Port       int       `json:"port"`
	User       string    `json:"user"`
	AuthMode   string    `json:"authMode"`
	Password   string    `json:"-"`
	PrivateKey string    `json:"-"`
	PassPhrase string    `json:"-"`
}

func (computeTerminalHost) TableName() string { return "hosts" }

func computeTerminalHosts() ([]computeTerminalHost, error) {
	var res []computeTerminalHost
	if global.AgentDB == nil {
		return nil, fmt.Errorf("agent database is unavailable")
	}
	err := global.AgentDB.Order("name asc").Find(&res).Error
	return res, err
}

func computeTerminalHostByID(id uint) (computeTerminalHost, error) {
	var h computeTerminalHost
	if global.AgentDB == nil {
		return h, fmt.Errorf("agent database is unavailable")
	}
	if err := global.AgentDB.Where("id = ?", id).First(&h).Error; err != nil {
		return h, fmt.Errorf("terminal host #%d no longer exists (Terminal → Hosts)", id)
	}
	return h, nil
}

// computeAgentKey returns the agent encryption key normalised the way the
// agent does it (padded with "u" / cut to 16 bytes): terminal credentials
// are encrypted by the agent, not by core.
func computeAgentKey() (string, error) {
	var setting model.Setting
	if global.AgentDB == nil {
		return "", fmt.Errorf("agent database is unavailable")
	}
	if err := global.AgentDB.Where("key = ?", "EncryptKey").First(&setting).Error; err != nil {
		return "", fmt.Errorf("read agent encrypt key: %w", err)
	}
	key := strings.TrimSpace(setting.Value)
	if key == "" {
		return "", fmt.Errorf("agent encrypt key is empty")
	}
	for len(key) < 16 {
		key += "u"
	}
	return key[:16], nil
}

func computeAgentDecrypt(cipher, key string) (string, error) {
	if cipher == "" {
		return "", nil
	}
	return encrypt.StringDecryptWithKey(cipher, key)
}

func computeAgentEncrypt(plain, key string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return encrypt.StringEncryptWithKey(plain, key)
}

// computeTerminalSig changes whenever the terminal host is edited, so the
// pooled connection is redialled with the new credentials.
func computeTerminalSig(t computeTerminalHost) string {
	return fmt.Sprintf("%d|%s|%d|%s|%s|%d", t.ID, t.Addr, t.Port, t.User, t.AuthMode, t.UpdatedAt.UnixNano())
}

func computeTerminalConnInfo(t computeTerminalHost) (ssh.ConnInfo, error) {
	info := ssh.ConnInfo{User: t.User, Addr: t.Addr, Port: t.Port, AuthMode: t.AuthMode, DialTimeOut: computeSSHDialTimeout}
	if info.Port == 0 {
		info.Port = 22
	}
	if info.User == "" {
		info.User = "root"
	}
	if t.AuthMode == "passwordless" {
		return info, nil
	}
	key, err := computeAgentKey()
	if err != nil {
		return info, err
	}
	switch t.AuthMode {
	case "key":
		pk, err := computeAgentDecrypt(t.PrivateKey, key)
		if err != nil {
			return info, fmt.Errorf("decrypt private key of terminal host %s: %w", t.Name, err)
		}
		pp, err := computeAgentDecrypt(t.PassPhrase, key)
		if err != nil {
			return info, fmt.Errorf("decrypt passphrase of terminal host %s: %w", t.Name, err)
		}
		info.PrivateKey, info.PassPhrase = []byte(pk), []byte(pp)
	default:
		pw, err := computeAgentDecrypt(t.Password, key)
		if err != nil {
			return info, fmt.Errorf("decrypt password of terminal host %s: %w", t.Name, err)
		}
		if pw == "" {
			return info, fmt.Errorf("terminal host %s has no saved password (enable \"remember password\" in Terminal → Hosts)", t.Name)
		}
		info.AuthMode, info.Password = "password", pw
	}
	return info, nil
}

// ---- panel hosts ----------------------------------------------------------

func computePanelNode(h model.ComputeHost) (model.ClusterNode, error) {
	if h.Source == model.ComputeSourceLocal {
		return localClusterNode(), nil
	}
	node, err := clusterNodeRepo.Get(repo.WithByID(h.RefID))
	if err != nil || node.ID == 0 {
		return node, fmt.Errorf("cluster node #%d no longer exists (Cluster → Nodes)", h.RefID)
	}
	return node, nil
}

// computePanelCall calls an agent API of a panel host (local socket or the
// signed remote API).
func computePanelCall(h model.ComputeHost, method, path string, body, out interface{}, timeout time.Duration) error {
	node, err := computePanelNode(h)
	if err != nil {
		return err
	}
	err = NewIClusterSyncService().(*ClusterSyncService).callNodeJSON(node, method, path, body, out, timeout)
	if err != nil && isMissingRouteErr(err) && strings.HasPrefix(path, "/api/v2/ai/compute/") {
		return fmt.Errorf("panel on %s is too old for compute scheduling, upgrade it first", h.Name)
	}
	return err
}

type computeExecRes struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
	TimedOut bool   `json:"timedOut"`
}

func computePanelExec(h model.ComputeHost, script string, timeout time.Duration) (string, error) {
	var res computeExecRes
	body := map[string]interface{}{"script": script, "timeoutSec": int(timeout.Seconds())}
	if err := computePanelCall(h, http.MethodPost, "/api/v2/ai/compute/exec", body, &res, timeout+15*time.Second); err != nil {
		return "", err
	}
	switch {
	case res.TimedOut:
		return res.Output, fmt.Errorf("command timeout")
	case res.ExitCode != 0:
		return res.Output, fmt.Errorf("exit status %d", res.ExitCode)
	}
	return res.Output, nil
}

// computePanelSample reads the fixed, read-only sampling scripts through
// the agent (not operation-logged, unlike exec).
func computePanelSample(h model.ComputeHost, tier string, timeout time.Duration) (string, error) {
	var res computeExecRes
	if err := computePanelCall(h, http.MethodPost, "/api/v2/ai/compute/sample", map[string]string{"tier": tier}, &res, timeout+10*time.Second); err != nil {
		return "", err
	}
	if res.TimedOut {
		return res.Output, fmt.Errorf("sample timeout")
	}
	return res.Output, nil
}

// computeExec runs a script on a compute host through the channel of its
// source. sudo only matters for SSH logins other than root (panels run
// their agent as root).
func computeExec(h model.ComputeHost, script string, sudo bool, timeout time.Duration) (string, error) {
	if h.IsPanel() {
		out, err := computePanelExec(h, script, timeout)
		if err != nil {
			global.LOG.Debugf("compute exec %s: %v", h.Name, err)
		}
		return out, err
	}
	return computeSSH.RunTimeout(h, script, sudo, timeout)
}

package model

import "time"

// ClusterNode is a registered remote 1Panel panel managed by this (master) panel.
// This is a local node registry only: nodes stay independent panels, no data is
// merged and no operation is proxied. It is unrelated to the paid xpack
// multi-node feature (core/utils/xpack) and to database primary-replica clusters.
type ClusterNode struct {
	BaseModel
	Name         string     `gorm:"not null;uniqueIndex" json:"name"`
	EntryURL     string     `gorm:"not null" json:"entryUrl"`
	Remark       string     `json:"remark"`
	GroupID      uint       `json:"groupId"`
	APIKeyCipher string     `gorm:"type:text;not null" json:"-"`
	KeyHint      string     `json:"keyHint"`
	Status       string     `gorm:"not null;default:unknown" json:"status"`
	LatencyMs    int64      `json:"latencyMs"`
	Version      string     `json:"version"`
	LastCheckAt  *time.Time `json:"lastCheckAt"`
	LastError    string     `gorm:"type:text" json:"lastError"`
	// Snapshot of the last successful aggregate refresh (GET
	// /dashboard/current/node + app/website counts). Zero values mean
	// "never refreshed successfully"; the frontend renders them as —.
	CpuUsedPercent  float64 `json:"cpuUsedPercent"`
	MemUsedPercent  float64 `json:"memUsedPercent"`
	AppCount        int     `json:"appCount"`
	WebsiteCount    int     `json:"websiteCount"`
	DiskUsedPercent float64 `json:"diskUsedPercent"`
	DiskTotal       uint64  `json:"diskTotal"`
	DiskUsed        uint64  `json:"diskUsed"`
	// Hardware snapshot from the last cluster monitor static sample:
	// hottest CPU sensor, accelerator count and the busiest / hottest
	// accelerator. GpuCount 0 = no GPU (the UI hides GPU columns).
	CpuTemperature float64 `json:"cpuTemperature"`
	GpuCount       int     `json:"gpuCount"`
	GpuUtilMax     float64 `json:"gpuUtilMax"`
	GpuTempMax     float64 `json:"gpuTempMax"`
	// Status of the most recent sync task targeting this node as a target
	// (success / partial / failed / — when never targeted).
	LastSyncStatus string     `json:"lastSyncStatus"`
	LastSyncAt     *time.Time `json:"lastSyncAt"`
	// Managed login lock of the node panel, refreshed on every check.
	// nil = unknown (never read, or the node is too old to support it).
	LoginLocked      *bool `json:"loginLocked"`
	LoginUnlockUntil int64 `json:"loginUnlockUntil"`
	// Mode selects how the master connects to this node:
	//   "panel"        - full 1Panel/XPanel node over HTTPS + API key (default)
	//   "agent"        - agent-only node over direct mTLS (no web panel)
	//   "agent-tunnel" - agent-only node over reverse WebSocket tunnel (NAT-friendly)
	Mode string `gorm:"not null;default:panel" json:"mode"`
	// HostID optionally links this cluster node to a local SSH host row.
	HostID uint `gorm:"index" json:"hostId"`
	// AgentPort is the direct mTLS port for mode="agent" (default 9999).
	AgentPort int `gorm:"default:9999" json:"agentPort"`
	// ProxyIDCipher stores the AES-GCM encrypted Proxy-Id for agent / agent-tunnel nodes.
	ProxyIDCipher string `gorm:"type:text" json:"-"`
}

func (ClusterNode) TableName() string { return "cluster_nodes" }

const (
	ClusterNodeStatusOnline  = "online"
	ClusterNodeStatusOffline = "offline"
	ClusterNodeStatusUnknown = "unknown"

	ClusterNodeModePanel       = "panel"
	ClusterNodeModeAgent       = "agent"
	ClusterNodeModeAgentTunnel = "agent-tunnel"
)

func (n ClusterNode) EffectiveMode() string {
	switch n.Mode {
	case ClusterNodeModeAgent, ClusterNodeModeAgentTunnel:
		return n.Mode
	default:
		return ClusterNodeModePanel
	}
}

func (n ClusterNode) IsAgentOnly() bool {
	m := n.EffectiveMode()
	return m == ClusterNodeModeAgent || m == ClusterNodeModeAgentTunnel
}

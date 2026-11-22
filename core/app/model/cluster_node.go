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
	// Status of the most recent sync task targeting this node as a target
	// (success / partial / failed / — when never targeted).
	LastSyncStatus string     `json:"lastSyncStatus"`
	LastSyncAt     *time.Time `json:"lastSyncAt"`
}

func (ClusterNode) TableName() string { return "cluster_nodes" }

const (
	ClusterNodeStatusOnline  = "online"
	ClusterNodeStatusOffline = "offline"
	ClusterNodeStatusUnknown = "unknown"
)

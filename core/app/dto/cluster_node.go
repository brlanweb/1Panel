package dto

import "time"

type ClusterNodeCreate struct {
	Name     string `json:"name" validate:"required"`
	EntryURL string `json:"entryUrl" validate:"required"`
	Remark   string `json:"remark"`
	GroupID  uint   `json:"groupId"`
	// Remote panel legacy API key. Stored AES-GCM encrypted, never returned.
	APIKey string `json:"apiKey" validate:"required"`
}

type ClusterNodeUpdate struct {
	ID       uint   `json:"id" validate:"required"`
	Name     string `json:"name" validate:"required"`
	EntryURL string `json:"entryUrl" validate:"required"`
	Remark   string `json:"remark"`
	GroupID  uint   `json:"groupId"`
	// Empty means "keep the stored key".
	APIKey string `json:"apiKey"`
}

type ClusterNodeSearch struct {
	SearchWithPage
	Status  string `json:"status"`
	GroupID uint   `json:"groupId"`
}

type ClusterNodeInfo struct {
	ID             uint       `json:"id"`
	Name           string     `json:"name"`
	EntryURL       string     `json:"entryUrl"`
	Remark         string     `json:"remark"`
	GroupID        uint       `json:"groupId"`
	KeyHint        string     `json:"keyHint"`
	Status         string     `json:"status"`
	LatencyMs      int64      `json:"latencyMs"`
	Version        string     `json:"version"`
	LastCheckAt    *time.Time `json:"lastCheckAt"`
	LastError      string     `json:"lastError"`
	CpuUsedPercent float64    `json:"cpuUsedPercent"`
	MemUsedPercent float64    `json:"memUsedPercent"`
	AppCount       int        `json:"appCount"`
	WebsiteCount   int        `json:"websiteCount"`
	LastSyncStatus string     `json:"lastSyncStatus"`
	LastSyncAt     *time.Time `json:"lastSyncAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type ClusterNodeOverview struct {
	Total   int64 `json:"total"`
	Online  int64 `json:"online"`
	Offline int64 `json:"offline"`
	Unknown int64 `json:"unknown"`
}

// ClusterNodeOption is one entry of the bottom node switcher: the master
// ("local") entry is added by the frontend, the rest come from ListOptions.
// Status uses the official switcher vocabulary (Healthy/Unhealthy/Unknown);
// Version is always empty for panel nodes (no version endpoint exists).
type ClusterNodeOption struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Addr    string `json:"addr"`
	Status  string `json:"status"`
	IsBound bool   `json:"isBound"`
	Version string `json:"version"`
}

// ClusterNodeSnapshot is one node's last aggregate refresh: live CPU/memory
// from the remote GET /dashboard/current/node plus app/website totals. A
// nil FetchError means the refresh succeeded; otherwise the snapshot fields
// are the last good values and FetchError explains the failure.
type ClusterNodeSnapshot struct {
	NodeID         uint    `json:"nodeId"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	CpuUsedPercent float64 `json:"cpuUsedPercent"`
	MemUsedPercent float64 `json:"memUsedPercent"`
	CpuTotal       int     `json:"cpuTotal"`
	MemTotal       uint64  `json:"memTotal"`
	AppCount       int     `json:"appCount"`
	WebsiteCount   int     `json:"websiteCount"`
	FetchError     string  `json:"fetchError"`
	RefreshedAt    string  `json:"refreshedAt"`
}

type ClusterAggregateOverview struct {
	ClusterNodeOverview
	TotalApps     int                   `json:"totalApps"`
	TotalWebsites int                   `json:"totalWebsites"`
	Nodes         []ClusterNodeSnapshot `json:"nodes"`
}

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
	ID              uint       `json:"id"`
	Name            string     `json:"name"`
	EntryURL        string     `json:"entryUrl"`
	Remark          string     `json:"remark"`
	GroupID         uint       `json:"groupId"`
	KeyHint         string     `json:"keyHint"`
	Status          string     `json:"status"`
	LatencyMs       int64      `json:"latencyMs"`
	Version         string     `json:"version"`
	LastCheckAt     *time.Time `json:"lastCheckAt"`
	LastError       string     `json:"lastError"`
	CpuUsedPercent  float64    `json:"cpuUsedPercent"`
	MemUsedPercent  float64    `json:"memUsedPercent"`
	AppCount        int        `json:"appCount"`
	WebsiteCount    int        `json:"websiteCount"`
	LastSyncStatus  string     `json:"lastSyncStatus"`
	LastSyncAt      *time.Time `json:"lastSyncAt"`
	DiskUsedPercent float64    `json:"diskUsedPercent"`
	DiskTotal       uint64     `json:"diskTotal"`
	DiskUsed        uint64     `json:"diskUsed"`
	CpuTemperature  float64    `json:"cpuTemperature"`
	GpuCount        int        `json:"gpuCount"`
	GpuUtilMax      float64    `json:"gpuUtilMax"`
	GpuTempMax      float64    `json:"gpuTempMax"`
	// Managed login lock snapshot: nil = never read / node too old.
	LoginLocked      *bool     `json:"loginLocked"`
	LoginUnlockUntil int64     `json:"loginUnlockUntil"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
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
	// Panel version of the node (SystemVersion of its settings) and its
	// disk usage summary (the mount of "/", or the largest mount).
	Version         string            `json:"version"`
	DiskUsedPercent float64           `json:"diskUsedPercent"`
	DiskTotal       uint64            `json:"diskTotal"`
	DiskUsed        uint64            `json:"diskUsed"`
	Disks           []ClusterDiskInfo `json:"disks"`
}

type ClusterDiskInfo struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"usedPercent"`
}

// ClusterNodeUpgradeReq starts a panel upgrade on a registered node.
type ClusterNodeUpgradeReq struct {
	ID uint `json:"id" validate:"required"`
	// Version to upgrade to; empty means the newest fork release.
	Version string `json:"version"`
	// Mode: "direct" (default) = the node downloads the package itself;
	// "master" = this panel downloads it once (cached per version / arch)
	// and pushes it to the node, for nodes without (fast) internet access.
	Mode string `json:"mode" validate:"omitempty,oneof=direct master"`
}

type ClusterNodeUpgradeStatus struct {
	ID             uint   `json:"id"`
	Stage          string `json:"stage"`
	Version        string `json:"version"`
	CurrentVersion string `json:"currentVersion"`
	Done           int64  `json:"done"`
	Total          int64  `json:"total"`
	Error          string `json:"error"`
	// Reachable is false while the remote panel restarts after installing.
	Reachable bool `json:"reachable"`
}

type ClusterAggregateOverview struct {
	ClusterNodeOverview
	TotalApps     int                   `json:"totalApps"`
	TotalWebsites int                   `json:"totalWebsites"`
	Nodes         []ClusterNodeSnapshot `json:"nodes"`
	// Local is the master panel itself, counted through its own agent
	// socket. LocalError is set when that count failed (-1 counts).
	Local      ClusterNodeSnapshot `json:"local"`
	LocalError string              `json:"localError"`
	// LatestVersion is the newest published fork release ("" when the
	// release channel could not be queried).
	LatestVersion string `json:"latestVersion"`
}

// ClusterManagedLoginReq is sent by the master to a node (API key only).
type ClusterManagedLoginReq struct {
	Locked        bool   `json:"locked"`
	UnlockMinutes int    `json:"unlockMinutes" validate:"min=0,max=10080"`
	Master        string `json:"master"`
}

// ClusterNodeLoginLockReq is the master-side batch request.
//
//	locked=true                    lock the node web login
//	locked=false, unlockMinutes>0  open temporarily, auto-lock afterwards
//	locked=false, unlockMinutes=0  remove the lock permanently
type ClusterNodeLoginLockReq struct {
	NodeIDs       []uint `json:"nodeIds" validate:"required,min=1"`
	Locked        bool   `json:"locked"`
	UnlockMinutes int    `json:"unlockMinutes" validate:"min=0,max=10080"`
}

type ClusterNodeLoginLockResult struct {
	NodeID      uint   `json:"nodeId"`
	Name        string `json:"name"`
	Success     bool   `json:"success"`
	Supported   bool   `json:"supported"`
	Locked      bool   `json:"locked"`
	UnlockUntil int64  `json:"unlockUntil"`
	Message     string `json:"message"`
}

// ClusterNodeJoinReq joins a Terminal → Hosts entry to the cluster: the
// panel is installed over SSH when missing, then a dedicated API key is
// created on it and the node is registered.
type ClusterNodeJoinReq struct {
	HostID uint `json:"hostId" validate:"required"`
	// Node name; empty = the terminal host name.
	Name    string `json:"name"`
	GroupID uint   `json:"groupId"`
	// IP / CIDR list (comma separated) allowed to use the key; empty = the
	// source IP the host sees for the master's SSH connection.
	MasterIP string `json:"masterIp"`
	// Address the master uses for the panel; empty = the SSH address.
	EntryHost     string `json:"entryHost"`
	InstallDocker bool   `json:"installDocker"`
}

type ClusterNodeJoinStatus struct {
	HostID    uint     `json:"hostId"`
	Stage     string   `json:"stage"`
	Running   bool     `json:"running"`
	Error     string   `json:"error"`
	Done      int64    `json:"done"`
	Total     int64    `json:"total"`
	Installed bool     `json:"installed"`
	NodeID    uint     `json:"nodeId"`
	NodeName  string   `json:"nodeName"`
	EntryURL  string   `json:"entryUrl"`
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	Logs      []string `json:"logs"`
}

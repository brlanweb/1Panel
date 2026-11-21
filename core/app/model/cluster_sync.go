package model

// ClusterSyncTask is one user-initiated resource-sync execution: one resource
// kind, one source node, one or more target nodes. Records are per-item rows
// in ClusterSyncRecord; the task row is the per-execution summary. Nothing
// here runs on a schedule — there is no background auto-sync.
type ClusterSyncTask struct {
	BaseModel
	Kind         string `gorm:"not null" json:"kind"` // file | image | app | cert
	SourceNodeID uint   `gorm:"not null" json:"sourceNodeId"`
	SourceName   string `json:"sourceName"`
	// TargetNodeIDs is stored as a JSON array string, e.g. "[2,3]".
	TargetNodeIDs string `gorm:"type:text" json:"targetNodeIds"`
	Status        string `gorm:"not null;default:running" json:"status"` // running | success | partial | failed
	Total         int    `json:"total"`
	Succeeded     int    `json:"succeeded"`
	Skipped       int    `json:"skipped"`
	Failed        int    `json:"failed"`
	// OverwriteConflicts records the user's explicit choice at execution
	// time. Default false: conflicting items are skipped and reported.
	OverwriteConflicts bool `json:"overwriteConflicts"`
	// Step is the stage currently running (e.g. "redis:7 → uk01: export
	// on source"), shown while the task runs so a slow stage is visible.
	Step string `json:"step"`
	// Byte progress of the running transfer stage (download / upload);
	// all zero when the current stage has no measurable progress.
	ProgressDone  int64 `json:"progressDone"`
	ProgressTotal int64 `json:"progressTotal"`
	ProgressRate  int64 `json:"progressRate"` // bytes per second
}

func (ClusterSyncTask) TableName() string { return "cluster_sync_tasks" }

const (
	ClusterSyncTaskRunning = "running"
	ClusterSyncTaskSuccess = "success"
	ClusterSyncTaskPartial = "partial"
	ClusterSyncTaskFailed  = "failed"
)

// ClusterSyncRecord is one (resource × target node) outcome of a sync task.
// Action "create" means the resource was transferred; "skip-same" means the
// target already had identical content; "skip-conflict" means the target had
// different content and was NOT overwritten; "failed" carries the reason.
type ClusterSyncRecord struct {
	BaseModel
	TaskID       uint   `gorm:"not null;index" json:"taskId"`
	Kind         string `gorm:"not null" json:"kind"`
	ResourceKey  string `gorm:"not null" json:"resourceKey"`
	ResourceName string `json:"resourceName"`
	SourceNodeID uint   `json:"sourceNodeId"`
	TargetNodeID uint   `gorm:"not null;index" json:"targetNodeId"`
	TargetName   string `json:"targetName"`
	Action       string `gorm:"not null" json:"action"` // create | skip-same | skip-conflict | failed
	Reason       string `gorm:"type:text" json:"reason"`
	DurationMs   int64  `json:"durationMs"`
}

func (ClusterSyncRecord) TableName() string { return "cluster_sync_records" }

const (
	ClusterSyncActionCreate       = "create"
	ClusterSyncActionSkipSame     = "skip-same"
	ClusterSyncActionSkipConflict = "skip-conflict"
	ClusterSyncActionFailed       = "failed"
)

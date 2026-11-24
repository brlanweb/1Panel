package dto

import "time"

const (
	ClusterSyncKindFile  = "file"
	ClusterSyncKindImage = "image"
	ClusterSyncKindApp   = "app"
	ClusterSyncKindCert  = "cert"
	// ClusterSyncKindPhp is a PHP extension template (name + extension list),
	// listed from the source panel's own template table
	// (POST /runtimes/php/extensions/search) and written to the target with
	// POST /runtimes/php/extensions/import (create / skip-same) or
	// /update (explicit overwrite). The official xpack push endpoint is NOT
	// used: it answers ErrXpackNotFound on open-source panels.
	ClusterSyncKindPhp = "php"
	// ClusterSyncKindModel is a vLLM model directory (Key = absolute dir).
	// It is copied file by file to the same path on the target and then
	// registered in the target's vLLM model list.
	ClusterSyncKindModel = "model"
)

// ClusterSyncResource is one normalized resource listed from a source node.
// Key is the stable identity used for conflict comparison (file path,
// image repo:tag, app install id, certificate domain).
type ClusterSyncResource struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Size   int64  `json:"size"`
	Extra  string `json:"extra"`
	// Meta carries display-only fields for the sync pages (file isDir /
	// symlink, image tags, app status, certificate expiry …).
	Meta map[string]interface{} `json:"meta,omitempty"`
}

type ClusterSyncResourceSearch struct {
	Kind         string `json:"kind" validate:"required,oneof=file image app cert php model"`
	SourceNodeID uint   `json:"sourceNodeId"` // 0 is the local (master) panel; `required` would reject it
	// File resources only: directory to list on the source node.
	Path string `json:"path"`
	// Search filters by name (file name, image name, app name, domain …).
	Search string `json:"search"`
	// SortOrder (files only): ascending | descending by name.
	SortOrder string `json:"sortOrder"`
	Page      int    `json:"page"`
	// PageSize caps the listing (default 100, max 500).
	PageSize int `json:"pageSize"`
}

// ClusterSyncResourcePage is one page of a node's resources. Path is the
// directory actually listed for files ("" when it does not exist).
type ClusterSyncResourcePage struct {
	Items []ClusterSyncResource `json:"items"`
	Total int64                 `json:"total"`
	Path  string                `json:"path"`
}

type ClusterSyncPreviewReq struct {
	Kind          string   `json:"kind" validate:"required,oneof=file image app cert php model"`
	SourceNodeID  uint     `json:"sourceNodeId"` // 0 is the local (master) panel; `required` would reject it
	TargetNodeIDs []uint   `json:"targetNodeIds" validate:"required,min=1"`
	ResourceKeys  []string `json:"resourceKeys" validate:"required,min=1"`
	// File resources only: directory the keys were listed from.
	Path string `json:"path"`
	// File resources only: directory on the target node (see ExecuteReq).
	TargetPath string `json:"targetPath"`
	// App resources only (see ExecuteReq).
	AppDBHost string `json:"appDbHost"`
}

type ClusterSyncPreviewItem struct {
	ResourceKey  string `json:"resourceKey"`
	ResourceName string `json:"resourceName"`
	TargetNodeID uint   `json:"targetNodeId"`
	TargetName   string `json:"targetName"`
	// Action the execution would take: create | skip-same | skip-conflict |
	// failed (preview could not be computed, see Reason).
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type ClusterSyncExecuteReq struct {
	Kind          string   `json:"kind" validate:"required,oneof=file image app cert php model"`
	SourceNodeID  uint     `json:"sourceNodeId"` // 0 is the local (master) panel; `required` would reject it
	TargetNodeIDs []uint   `json:"targetNodeIds" validate:"required,min=1"`
	ResourceKeys  []string `json:"resourceKeys" validate:"required,min=1"`
	// File resources only: directory the keys were listed from.
	Path string `json:"path"`
	// File resources only: directory on the target node to transfer into.
	// Empty keeps the source directory (same path on the target).
	TargetPath string `json:"targetPath"`
	// App resources only: database service (by name, as listed on the
	// targets) the migrated app connects to; required for linkDB apps.
	AppDBHost string `json:"appDbHost"`
	// Explicit opt-in: overwrite target content that differs. Default false
	// (conflicts are skipped and reported, never silently overwritten).
	OverwriteConflicts bool `json:"overwriteConflicts"`
}

// ClusterSyncAppDBReq asks for the database services available on every
// given node (app-transfer dependency selector).
type ClusterSyncAppDBReq struct {
	NodeIDs []uint `json:"nodeIds" validate:"required,min=1"`
	// Types is a comma-separated list (default mysql,mariadb,postgresql).
	Types string `json:"types"`
}

type ClusterSyncAppDB struct {
	Type     string `json:"type"`
	Database string `json:"database"`
}

// ClusterSyncFileDelete deletes one file / directory on a node from the
// file-transfer page (moved to the node's recycle bin when enabled).
type ClusterSyncFileDelete struct {
	NodeID uint   `json:"nodeId"`
	Path   string `json:"path" validate:"required"`
	IsDir  bool   `json:"isDir"`
}

type ClusterSyncTaskInfo struct {
	ID                 uint      `json:"id"`
	Kind               string    `json:"kind"`
	SourceNodeID       uint      `json:"sourceNodeId"`
	SourceName         string    `json:"sourceName"`
	TargetNodeIDs      string    `json:"targetNodeIds"`
	Status             string    `json:"status"`
	Total              int       `json:"total"`
	Succeeded          int       `json:"succeeded"`
	Skipped            int       `json:"skipped"`
	Failed             int       `json:"failed"`
	OverwriteConflicts bool      `json:"overwriteConflicts"`
	Step               string    `json:"step"`
	ProgressDone       int64     `json:"progressDone"`
	ProgressTotal      int64     `json:"progressTotal"`
	ProgressRate       int64     `json:"progressRate"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type ClusterSyncRecordInfo struct {
	ID           uint      `json:"id"`
	TaskID       uint      `json:"taskId"`
	Kind         string    `json:"kind"`
	ResourceKey  string    `json:"resourceKey"`
	ResourceName string    `json:"resourceName"`
	SourceNodeID uint      `json:"sourceNodeId"`
	TargetNodeID uint      `json:"targetNodeId"`
	TargetName   string    `json:"targetName"`
	Action       string    `json:"action"`
	Reason       string    `json:"reason"`
	DurationMs   int64     `json:"durationMs"`
	CreatedAt    time.Time `json:"createdAt"`
}

type ClusterSyncTaskSearch struct {
	SearchWithPage
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

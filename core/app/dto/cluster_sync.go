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
}

type ClusterSyncResourceSearch struct {
	Kind         string `json:"kind" validate:"required,oneof=file image app cert php"`
	SourceNodeID uint   `json:"sourceNodeId" validate:"required"`
	// File resources only: directory to list on the source node.
	Path string `json:"path"`
	Page int    `json:"page"`
	// PageSize caps the listing (default 100, max 500).
	PageSize int `json:"pageSize"`
}

type ClusterSyncPreviewReq struct {
	Kind          string   `json:"kind" validate:"required,oneof=file image app cert php"`
	SourceNodeID  uint     `json:"sourceNodeId" validate:"required"`
	TargetNodeIDs []uint   `json:"targetNodeIds" validate:"required,min=1"`
	ResourceKeys  []string `json:"resourceKeys" validate:"required,min=1"`
	// File resources only: directory the keys were listed from.
	Path string `json:"path"`
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
	Kind          string   `json:"kind" validate:"required,oneof=file image app cert php"`
	SourceNodeID  uint     `json:"sourceNodeId" validate:"required"`
	TargetNodeIDs []uint   `json:"targetNodeIds" validate:"required,min=1"`
	ResourceKeys  []string `json:"resourceKeys" validate:"required,min=1"`
	// File resources only: directory the keys were listed from.
	Path string `json:"path"`
	// Explicit opt-in: overwrite target content that differs. Default false
	// (conflicts are skipped and reported, never silently overwritten).
	OverwriteConflicts bool `json:"overwriteConflicts"`
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

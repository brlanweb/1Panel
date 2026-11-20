package dto

// Cluster DB replication DTOs. Node ids follow the sync convention: 0 is
// this (master) panel, anything else is a registered cluster node.

const (
	ClusterDbTypeMysql      = "mysql"
	ClusterDbTypeMariadb    = "mariadb"
	ClusterDbTypePostgresql = "postgresql"
)

func ValidClusterDbType(dbType string) bool {
	switch dbType {
	case ClusterDbTypeMysql, ClusterDbTypeMariadb, ClusterDbTypePostgresql:
		return true
	}
	return false
}

type ClusterDbReplicationCreate struct {
	Name            string `json:"name" validate:"required"`
	DBType          string `json:"dbType" validate:"required"`
	MasterNodeID    uint   `json:"masterNodeId"`
	MasterInstance  string `json:"masterInstance" validate:"required"`
	MasterHost      string `json:"masterHost" validate:"required"`
	MasterPort      uint   `json:"masterPort" validate:"required"`
	Database        string `json:"database" validate:"required"`
	ReplicaNodeID   uint   `json:"replicaNodeId"`
	ReplicaInstance string `json:"replicaInstance" validate:"required"`
	ReplicaDatabase string `json:"replicaDatabase" validate:"required"`
	// Replication account: the user creates it manually on the master
	// (CREATE USER ... + REPLICATION privilege) and enters the same
	// credentials here; the password is stored AES-GCM encrypted.
	ReplUsername string `json:"replUsername" validate:"required"`
	ReplPassword string `json:"replPassword" validate:"required"`
}

type ClusterDbReplicationUpdate struct {
	ID           uint   `json:"id" validate:"required"`
	Name         string `json:"name"`
	ReplUsername string `json:"replUsername"`
	ReplPassword string `json:"replPassword"`
}

type ClusterDbReplicationInfo struct {
	ID                   uint   `json:"id"`
	Name                 string `json:"name"`
	DBType               string `json:"dbType"`
	MasterNodeID         uint   `json:"masterNodeId"`
	MasterName           string `json:"masterName"`
	MasterInstance       string `json:"masterInstance"`
	MasterHost           string `json:"masterHost"`
	MasterPort           uint   `json:"masterPort"`
	Database             string `json:"database"`
	ReplicaNodeID        uint   `json:"replicaNodeId"`
	ReplicaName          string `json:"replicaName"`
	ReplicaInstance      string `json:"replicaInstance"`
	ReplicaDatabase      string `json:"replicaDatabase"`
	ReplUsername         string `json:"replUsername"`
	Status               string `json:"status"`
	SetupStep            string `json:"setupStep"`
	ManualSteps          string `json:"manualSteps"`
	Preflight            string `json:"preflight"`
	MasterServerID       int    `json:"masterServerId"`
	ReplicaServerID      int    `json:"replicaServerId"`
	MasterBinlogFile     string `json:"masterBinlogFile"`
	MasterBinlogPosition string `json:"masterBinlogPosition"`
	SnapshotStatus       string `json:"snapshotStatus"`
	SnapshotDetail       string `json:"snapshotDetail"`
	LastDelaySeconds     *int   `json:"lastDelaySeconds"`
	LastError            string `json:"lastError"`
	LastCheckedAt        string `json:"lastCheckedAt"`
	LastConfirmedAt      string `json:"lastConfirmedAt"`
	FailoverSuggested    bool   `json:"failoverSuggested"`
	CreatedAt            string `json:"createdAt"`
	UpdatedAt            string `json:"updatedAt"`
}

type ClusterDbFailoverInfo struct {
	ID            uint   `json:"id"`
	ReplicationID uint   `json:"replicationId"`
	FromNodeID    uint   `json:"fromNodeId"`
	FromName      string `json:"fromName"`
	ToNodeID      uint   `json:"toNodeId"`
	ToName        string `json:"toName"`
	Reason        string `json:"reason"`
	Repointed     int    `json:"repointed"`
	CreatedAt     string `json:"createdAt"`
}

// ClusterDbPreflightCheck is one preflight result row. Status: pass |
// fail | warn | manual. "manual" means the panel cannot check this item
// through any existing API (e.g. replica lag, SSH tunnel) and the user
// must verify it by hand.
type ClusterDbPreflightCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// ClusterDbManualStep is one manual step with exact SQL/command text.
// Done is ticked by the user in the UI; the panel never claims a manual
// step is done until the user confirms it.
type ClusterDbManualStep struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	SQL   string `json:"sql"`
	Done  bool   `json:"done"`
}

// ClusterDbInstance is one database instance (installed app) on a node,
// from GET /api/v2/databases/db/list/:type.
type ClusterDbInstance struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Port    uint   `json:"port"`
}

// ClusterDbSchema is one database (schema) inside an instance, from
// POST /api/v2/databases/db/search.
type ClusterDbSchema struct {
	Name string `json:"name"`
}

type ClusterDbInstancesReq struct {
	NodeID uint   `json:"nodeId"`
	DBType string `json:"dbType" validate:"required"`
}

type ClusterDbSchemasReq struct {
	NodeID   uint   `json:"nodeId"`
	DBType   string `json:"dbType" validate:"required"`
	Instance string `json:"instance" validate:"required"`
}

type ClusterDbIDReq struct {
	ID uint `json:"id" validate:"required"`
}

type ClusterDbConfirmReq struct {
	ID uint `json:"id" validate:"required"`
	// ManualStepKeys lists the manual step keys the user confirms done.
	ManualStepKeys []string `json:"manualStepKeys"`
}

type ClusterDbPromoteReq struct {
	ID     uint   `json:"id" validate:"required"`
	Reason string `json:"reason"`
	// ManualStepKeys lists the manual step keys the user confirms done
	// (STOP REPLICA / RESET REPLICA ALL, or DROP SUBSCRIPTION).
	ManualStepKeys []string `json:"manualStepKeys"`
}

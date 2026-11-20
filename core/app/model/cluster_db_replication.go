package model

// ClusterDbReplication is one registered cross-server database replication
// topology: a single database on a master node replicated one-way to a
// replica node. The master panel only stores topology + credentials; it
// never holds the data itself.
//
// Replication itself is MySQL GTID + binlog (ROW) or PostgreSQL logical
// replication. Several steps cannot be done through any existing panel API
// (creating a global REPLICATION user, CHANGE REPLICATION SOURCE TO /
// START REPLICA, CREATE PUBLICATION / SUBSCRIPTION, SHOW REPLICA STATUS):
// those are explicit manual steps with exact SQL shown in the UI, and the
// topology records which of them the user has confirmed. Automated steps
// (config-file edits + instance restart, snapshot backup/transfer/restore,
// read-only flip) only use existing agent endpoints reached through the
// cluster node dispatcher.
type ClusterDbReplication struct {
	BaseModel
	Name string `gorm:"not null" json:"name"`
	// DBType is mysql | mariadb | postgresql. Mixed-engine chains are
	// rejected at creation (MySQL and MariaDB GTIDs do not interoperate).
	DBType string `gorm:"not null" json:"dbType"`
	// MasterNodeID / ReplicaNodeID are cluster node ids; 0 is this (master)
	// panel, the same convention as resource sync.
	MasterNodeID   uint   `gorm:"not null" json:"masterNodeId"`
	MasterName     string `json:"masterName"`
	MasterInstance string `gorm:"not null" json:"masterInstance"`
	// MasterHost is the address the replica's database connects to: the
	// master panel entry host. MasterPort is the database's published port.
	MasterHost      string `gorm:"not null" json:"masterHost"`
	MasterPort      uint   `gorm:"not null" json:"masterPort"`
	Database        string `gorm:"not null" json:"database"`
	ReplicaNodeID   uint   `gorm:"not null" json:"replicaNodeId"`
	ReplicaName     string `json:"replicaName"`
	ReplicaInstance string `gorm:"not null" json:"replicaInstance"`
	ReplicaDatabase string `gorm:"not null" json:"replicaDatabase"`
	// Replication account, created manually on the master (the panel user
	// API refuses global privileges). The password is AES-GCM encrypted
	// with the same key helper as cluster node API keys; it is never
	// written to logs and only rendered back inside the manual SQL shown
	// to the panel admin.
	ReplUsername       string `gorm:"not null" json:"replUsername"`
	ReplPasswordCipher string `gorm:"type:text" json:"-"`
	// Status: draft | ready | configuring | snapshot | awaiting_replication
	// | running | error | failed_over. "running" means the user confirmed
	// the manual replication-start SQL executed and replication is up;
	// the panel cannot read replica lag through any existing API, so
	// LastDelaySeconds stays null (shown as —) unless a future API
	// provides it.
	Status string `gorm:"not null;default:draft" json:"status"`
	// SetupStep records the last completed automated step
	// ("" | preflight | master_config | snapshot | manual_pending).
	SetupStep string `json:"setupStep"`
	// ManualSteps is a JSON array of {key, title, sql, done} for the
	// manual steps of the current phase, so the UI can show exact SQL
	// and the user can tick each step off.
	ManualSteps string `gorm:"type:text" json:"manualSteps"`
	// Preflight is a JSON array of {name, status, detail} from the last
	// preflight run (status: pass | fail | warn | manual).
	Preflight string `gorm:"type:text" json:"preflight"`
	// MasterServerID / ReplicaServerID are the server-id values written
	// to (or already present in) the two my.cnf files. PostgreSQL
	// topologies leave them 0.
	MasterServerID  int `json:"masterServerId"`
	ReplicaServerID int `json:"replicaServerId"`
	// MasterBinlogFile / MasterBinlogPosition are the last observed
	// master binlog position (MySQL LoadStatus); empty/"OFF" means the
	// binlog is not active.
	MasterBinlogFile     string `json:"masterBinlogFile"`
	MasterBinlogPosition string `json:"masterBinlogPosition"`
	// Snapshot tracks the background snapshot job: idle | running |
	// done | failed, plus a short human-readable detail.
	SnapshotStatus string `json:"snapshotStatus"`
	SnapshotDetail string `gorm:"type:text" json:"snapshotDetail"`
	// LastDelaySeconds is nil when unknown (no API exposes replica lag).
	LastDelaySeconds *int   `json:"lastDelaySeconds"`
	LastError        string `gorm:"type:text" json:"lastError"`
	LastCheckedAt    string `json:"lastCheckedAt"`
	LastConfirmedAt  string `json:"lastConfirmedAt"`
	// FailoverSuggested is set by RefreshStatus when the master node or
	// master database is unreachable: detection + alert only, the panel
	// never promotes a replica on its own (auto-promotion can split
	// brain under a network partition).
	FailoverSuggested bool `json:"failoverSuggested"`
}

func (ClusterDbReplication) TableName() string { return "cluster_db_replications" }

const (
	ClusterDbReplStatusDraft               = "draft"
	ClusterDbReplStatusReady               = "ready"
	ClusterDbReplStatusConfiguring         = "configuring"
	ClusterDbReplStatusSnapshot            = "snapshot"
	ClusterDbReplStatusAwaitingReplication = "awaiting_replication"
	ClusterDbReplStatusRunning             = "running"
	ClusterDbReplStatusError               = "error"
	ClusterDbReplStatusFailedOver          = "failed_over"
)

// ClusterDbFailover is one manual failover event: a replica promoted to
// master, with the other replicas of the same old master re-pointed at it.
type ClusterDbFailover struct {
	BaseModel
	ReplicationID uint   `gorm:"not null;index" json:"replicationId"`
	FromNodeID    uint   `json:"fromNodeId"`
	FromName      string `json:"fromName"`
	ToNodeID      uint   `json:"toNodeId"`
	ToName        string `json:"toName"`
	Reason        string `gorm:"type:text" json:"reason"`
	Repointed     int    `json:"repointed"`
}

func (ClusterDbFailover) TableName() string { return "cluster_db_failovers" }

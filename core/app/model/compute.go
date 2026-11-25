package model

import "time"

// Compute scheduling ("算力调度") models.
//
// The compute module owns no host credentials. Every compute host refers to
// a host that is already managed elsewhere in the panel:
//
//   - source "terminal": a host of Terminal → Hosts (agent.db `hosts`),
//     collected and operated agentless over a pooled SSH connection that
//     uses the credentials stored there;
//   - source "node": a registered cluster node (panel installed), reached
//     through its signed API;
//   - source "local": this (master) panel, reached over the agent socket.
//
// Realtime metrics are kept in memory only; the tables below hold
// inventory, topology, templates, instances, jobs and policies, so the
// write rate stays tiny even with dozens of hosts.

// ComputeHost is one GPU machine (e.g. a DGX GB10).
type ComputeHost struct {
	BaseModel
	Name string `gorm:"not null;uniqueIndex" json:"name"`
	// Addr is the management address: used for service endpoints and to
	// recognise the management NIC during topology discovery. Empty means
	// "auto" (filled from the host's default-route address).
	Addr   string `gorm:"not null;default:''" json:"addr"`
	Source string `gorm:"not null;default:terminal;index" json:"source"` // terminal / node / local
	// RefID is agent.db hosts.id (terminal) or cluster_nodes.id (node).
	RefID uint `gorm:"not null;default:0;index" json:"refId"`
	// Mode is derived from Source: agentless (terminal) / agent (node, local).
	Mode   string `gorm:"not null;default:agentless" json:"mode"`
	Labels string `json:"labels"`
	Remark string `json:"remark"`
	// Schedulable=false cordons the host: the scheduler never places new
	// instances on it (existing ones keep running).
	Schedulable bool `gorm:"not null;default:true" json:"schedulable"`

	Status      string     `gorm:"not null;default:unknown" json:"status"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`
	LastError   string     `gorm:"type:text" json:"lastError"`
	Hostname    string     `json:"hostname"`
	Arch        string     `json:"arch"`
	OS          string     `json:"os"`
	Driver      string     `json:"driver"`
	GpuModel    string     `json:"gpuModel"`
	GpuCount    int        `json:"gpuCount"`
	CpuCores    int        `json:"cpuCores"`
	MemTotal    uint64     `json:"memTotal"`
	UnifiedMem  bool       `json:"unifiedMem"`
	AssetSyncAt *time.Time `json:"assetSyncAt"`
}

func (ComputeHost) TableName() string { return "compute_hosts" }

const (
	ComputeHostOnline  = "online"
	ComputeHostOffline = "offline"
	ComputeHostUnknown = "unknown"

	ComputeModeAgentless = "agentless"
	ComputeModeAgent     = "agent"

	ComputeSourceTerminal = "terminal"
	ComputeSourceNode     = "node"
	ComputeSourceLocal    = "local"

	// Runtime backends: "script" renders the launch/stop templates and runs
	// them on every host; "vllm" creates a deployment in AI → vLLM of the
	// (single, panel-managed) host, so it is managed there as well.
	ComputeBackendScript = "script"
	ComputeBackendVllm   = "vllm"
)

// IsPanel reports whether the host runs a panel (node or local).
func (h ComputeHost) IsPanel() bool {
	return h.Source == ComputeSourceNode || h.Source == ComputeSourceLocal
}

// ComputeFabricLink is one high-speed point-to-point link (optical / RoCE)
// discovered from the neighbour table or entered by hand. Links that touch
// each other form a fabric domain; today every domain is a 2-host pair,
// after an optical switch is installed a domain simply grows.
type ComputeFabricLink struct {
	BaseModel
	Domain     string     `gorm:"index" json:"domain"`
	HostAID    uint       `gorm:"not null;index" json:"hostAId"`
	HostBID    uint       `gorm:"not null;index" json:"hostBId"`
	IfaceA     string     `json:"ifaceA"`
	IfaceB     string     `json:"ifaceB"`
	IPA        string     `json:"ipA"`
	IPB        string     `json:"ipB"`
	RdmaA      string     `json:"rdmaA"`
	RdmaB      string     `json:"rdmaB"`
	RateGbps   int        `json:"rateGbps"`
	State      string     `json:"state"`
	Source     string     `gorm:"not null;default:auto" json:"source"` // auto / manual
	LastSeenAt *time.Time `json:"lastSeenAt"`
}

func (ComputeFabricLink) TableName() string { return "compute_fabric_links" }

// ComputeRuntime is an inference engine template (model independent).
// LaunchTemplate / StopTemplate are Go text/template shell scripts run on
// each host of an instance; see compute_template.go for the variables.
type ComputeRuntime struct {
	BaseModel
	Name   string `gorm:"not null;uniqueIndex" json:"name"`
	Engine string `gorm:"not null;default:vllm" json:"engine"`
	// Device is the accelerator family the image is built for (gpu / npu);
	// placement only considers hosts with that kind of accelerator.
	Device string `gorm:"not null;default:gpu" json:"device"`
	// Backend: script (templates over SSH / panel API) or vllm (AI → vLLM
	// deployment on a panel host, single host only).
	Backend        string `gorm:"not null;default:script" json:"backend"`
	Image          string `json:"image"`
	MultiNode      bool   `json:"multiNode"`
	LaunchTemplate string `gorm:"type:text" json:"launchTemplate"`
	StopTemplate   string `gorm:"type:text" json:"stopTemplate"`
	HealthPath     string `gorm:"not null;default:/health" json:"healthPath"`
	MetricsPath    string `gorm:"not null;default:/metrics" json:"metricsPath"`
	Builtin        bool   `json:"builtin"`
	Remark         string `json:"remark"`
}

func (ComputeRuntime) TableName() string { return "compute_runtimes" }

// ComputeProfile binds a model to a runtime: what to serve, on how many
// hosts, with which resources and arguments.
type ComputeProfile struct {
	BaseModel
	Name       string `gorm:"not null;uniqueIndex" json:"name"`
	RuntimeID  uint   `gorm:"not null" json:"runtimeId"`
	ServedName string `gorm:"not null;index" json:"servedName"`
	ModelPath  string `gorm:"not null" json:"modelPath"`
	NodeCount  int    `gorm:"not null;default:1" json:"nodeCount"`
	NeedFabric bool   `json:"needFabric"`
	MinMemGB   int    `json:"minMemGb"`
	Port       int    `gorm:"not null;default:8000" json:"port"`
	ExtraArgs  string `gorm:"type:text" json:"extraArgs"`
	Env        string `gorm:"type:text" json:"env"`
	// StartTimeoutSec bounds "load weights + health ok"; large models on
	// unified memory can take a long time.
	StartTimeoutSec int    `gorm:"not null;default:1800" json:"startTimeoutSec"`
	Remark          string `json:"remark"`
}

func (ComputeProfile) TableName() string { return "compute_profiles" }

// ComputeInstance is one serving endpoint (single host or a multi-host
// group whose head exposes the API).
type ComputeInstance struct {
	BaseModel
	Name          string `gorm:"not null;uniqueIndex" json:"name"`
	ProfileID     uint   `json:"profileId"`
	ServedName    string `gorm:"not null;index" json:"servedName"`
	HeadHostID    uint   `gorm:"not null" json:"headHostId"`
	HostIDs       string `gorm:"not null" json:"hostIds"` // comma separated, head first
	Port          int    `gorm:"not null" json:"port"`
	Endpoint      string `gorm:"not null" json:"endpoint"`
	APIKeyCipher  string `gorm:"type:text" json:"-"`
	ContainerName string `json:"containerName"`
	// VllmDeploymentID is the AI → vLLM deployment on the head host that
	// backs this instance (runtime backend "vllm"); 0 otherwise.
	VllmDeploymentID uint `json:"vllmDeploymentId"`
	// Adopted instances were found running and are only observed: the panel
	// never restarts them unless the operator takes them over explicitly.
	Adopted bool   `json:"adopted"`
	Status  string `gorm:"not null;default:unknown" json:"status"`
	// Routing switches used by the model router.
	RouteEnabled bool   `gorm:"not null;default:true" json:"routeEnabled"`
	Weight       int    `gorm:"not null;default:100" json:"weight"`
	LastError    string `gorm:"type:text" json:"lastError"`
}

func (ComputeInstance) TableName() string { return "compute_instances" }

const (
	ComputeInstStarting = "starting"
	ComputeInstRunning  = "running"
	ComputeInstDraining = "draining"
	ComputeInstStopped  = "stopped"
	ComputeInstFailed   = "failed"
	ComputeInstUnknown  = "unknown"
)

// ComputeJob is a long running, step-by-step operation with rollback.
type ComputeJob struct {
	BaseModel
	Type       string     `gorm:"not null;index" json:"type"` // deploy/switch/stop/start/remove/scaleDown
	Status     string     `gorm:"not null;index" json:"status"`
	ServedName string     `json:"servedName"`
	InstanceID uint       `json:"instanceId"`
	ProfileID  uint       `json:"profileId"`
	HostIDs    string     `json:"hostIds"`
	Source     string     `json:"source"` // manual / suggestion / auto
	Reason     string     `gorm:"type:text" json:"reason"`
	Steps      string     `gorm:"type:text" json:"steps"` // JSON []ComputeJobStep
	Error      string     `gorm:"type:text" json:"error"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

func (ComputeJob) TableName() string { return "compute_jobs" }

const (
	ComputeJobPending    = "pending"
	ComputeJobRunning    = "running"
	ComputeJobSuccess    = "success"
	ComputeJobFailed     = "failed"
	ComputeJobRolledBack = "rolledback"
)

// ComputeConsumer is a caller of the model router (new-api, a VPS service…).
type ComputeConsumer struct {
	BaseModel
	Name             string     `gorm:"not null;uniqueIndex" json:"name"`
	KeyHash          string     `gorm:"not null;uniqueIndex" json:"-"`
	KeyHint          string     `json:"keyHint"`
	AllowedModels    string     `json:"allowedModels"` // comma separated, empty = all
	RPM              int        `json:"rpm"`           // 0 = unlimited
	Enabled          bool       `gorm:"not null;default:true" json:"enabled"`
	Requests         int64      `json:"requests"`
	Failures         int64      `json:"failures"`
	PromptTokens     int64      `json:"promptTokens"`
	CompletionTokens int64      `json:"completionTokens"`
	LastUsedAt       *time.Time `json:"lastUsedAt"`
	Remark           string     `json:"remark"`
}

func (ComputeConsumer) TableName() string { return "compute_consumers" }

// ComputePolicy is the slow-scheduling policy of one served model. With
// AccountID set it is a "compute pool": an AI → Model Services entry (on
// the local agent) that points at the model router, so the pool is used
// like any other model service.
type ComputePolicy struct {
	BaseModel
	// Name is the model service name of the pool (empty for plain policies).
	Name       string `gorm:"not null;default:''" json:"name"`
	AccountID  uint   `gorm:"not null;default:0" json:"accountId"`  // agent_provider_accounts.id (local agent)
	ConsumerID uint   `gorm:"not null;default:0" json:"consumerId"` // router key used by that service
	ServedName string `gorm:"not null;uniqueIndex" json:"servedName"`
	ProfileID    uint   `json:"profileId"`                            // profile used when scaling up
	Mode         string `gorm:"not null;default:suggest" json:"mode"` // off / suggest / auto
	MinInstances int    `json:"minInstances"`
	MaxInstances int    `gorm:"not null;default:1" json:"maxInstances"`
	HotSpare     int    `json:"hotSpare"`
	Priority     int    `gorm:"not null;default:50" json:"priority"`
	Preemptible  bool   `json:"preemptible"`
	// Scale up when the average waiting queue per instance stays above
	// QueueHigh (or KV cache usage above KVHigh) for SustainRounds rounds.
	QueueHigh float64 `gorm:"not null;default:4" json:"queueHigh"`
	KVHigh    float64 `gorm:"not null;default:0.92" json:"kvHigh"`
	// TTFTHigh (ms, 0 = ignored): scale up when the request-weighted mean
	// time to first token of the model stays above it as well.
	TTFTHigh      float64 `gorm:"not null;default:0" json:"ttftHigh"`
	SustainRounds int     `gorm:"not null;default:3" json:"sustainRounds"`
	// Scale down when no request ran for IdleMinutes.
	IdleMinutes int `gorm:"not null;default:30" json:"idleMinutes"`
	CooldownSec int `gorm:"not null;default:600" json:"cooldownSec"`
}

func (ComputePolicy) TableName() string { return "compute_policies" }

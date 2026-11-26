package dto

import "github.com/1Panel-dev/1Panel/core/app/model"

// ---- hosts ---------------------------------------------------------------

// ComputeHostImportReq brings existing hosts under compute scheduling.
// Source terminal: RefIDs are Terminal → Hosts ids; node: cluster node ids;
// local: this panel (RefIDs ignored).
type ComputeHostImportReq struct {
	Source string `json:"source" validate:"required,oneof=terminal node local"`
	RefIDs []uint `json:"refIds"`
	Labels string `json:"labels" validate:"max=255"`
}

type ComputeHostImportRes struct {
	Imported int    `json:"imported"`
	Error    string `json:"error"`
}

// ComputeHostReq edits the scheduling attributes of a compute host. The
// access itself (address, port, credentials) is edited where the host is
// managed (Terminal → Hosts / Cluster → Nodes).
type ComputeHostReq struct {
	ID          uint   `json:"id" validate:"required"`
	Name        string `json:"name" validate:"required,max=64"`
	Addr        string `json:"addr" validate:"max=255"`
	Labels      string `json:"labels" validate:"max=255"`
	Remark      string `json:"remark" validate:"max=255"`
	Schedulable *bool  `json:"schedulable"`
}

// ComputeHostCandidate is a host that can be imported.
type ComputeHostCandidate struct {
	Source   string `json:"source"`
	RefID    uint   `json:"refId"`
	Name     string `json:"name"`
	Addr     string `json:"addr"`
	Detail   string `json:"detail"` // user@addr:port / entry URL / version
	Status   string `json:"status"`
	Imported bool   `json:"imported"`
	HostID   uint   `json:"hostId"`
}

type ComputeSchedulableReq struct {
	ID          uint `json:"id" validate:"required"`
	Schedulable bool `json:"schedulable"`
}

// ComputeGPU is one accelerator: an NVIDIA GPU or an Ascend NPU chip.
type ComputeGPU struct {
	Kind     string  `json:"kind"` // gpu / npu
	Index    int     `json:"index"`
	Card     int     `json:"card"` // NPU card id (npu-smi "NPU" column)
	Chip     int     `json:"chip"` // NPU chip id on the card
	Name     string  `json:"name"`
	Health   string  `json:"health"` // NPU: OK / Warning / Alarm / Critical
	Util     float64 `json:"util"`
	Temp     float64 `json:"temp"`
	Power    float64 `json:"power"`
	MemUsed  uint64  `json:"memUsed"`  // bytes
	MemTotal uint64  `json:"memTotal"` // bytes
	Unified  bool    `json:"unified"`
}

type ComputeNic struct {
	Name      string  `json:"name"`
	State     string  `json:"state"`
	SpeedMbps int64   `json:"speedMbps"`
	IP        string  `json:"ip"`
	RxRate    float64 `json:"rxRate"` // bytes/s
	TxRate    float64 `json:"txRate"`
}

type ComputeRdma struct {
	Dev      string  `json:"dev"`
	Port     string  `json:"port"`
	NetDev   string  `json:"netDev"`
	State    string  `json:"state"`
	RateGbps int     `json:"rateGbps"`
	RxRate   float64 `json:"rxRate"` // bytes/s
	TxRate   float64 `json:"txRate"`
	// Error counters: totals since boot and the increase since the
	// previous sample (non-zero deltas raise a fabric warning).
	RcvErrors       uint64 `json:"rcvErrors"`
	SymbolErrors    uint64 `json:"symbolErrors"`
	LinkDowned      uint64 `json:"linkDowned"`
	XmitDiscards    uint64 `json:"xmitDiscards"`
	LinkErrRecovery uint64 `json:"linkErrRecovery"`
	ErrDelta        uint64 `json:"errDelta"`
}

type ComputeProcess struct {
	Pid        int    `json:"pid"`
	Engine     string `json:"engine"`
	Port       int    `json:"port"`
	Model      string `json:"model"`
	ServedName string `json:"servedName"`
	Cmd        string `json:"cmd"`
	InstanceID uint   `json:"instanceId"` // 0 = not registered yet
	// Worker rank of a multi-host group: no API, adopt its head instead.
	Worker bool `json:"worker"`
}

type ComputeHostRealtime struct {
	Online         bool             `json:"online"`
	Error          string           `json:"error"`
	SampledAt      int64            `json:"sampledAt"`
	CpuPercent     float64          `json:"cpuPercent"`
	Load1          float64          `json:"load1"`
	Cores          int              `json:"cores"`
	MemTotal       uint64           `json:"memTotal"`
	MemAvailable   uint64           `json:"memAvailable"`
	MemUsedPercent float64          `json:"memUsedPercent"`
	SwapUsed       uint64           `json:"swapUsed"`
	DiskTotal      uint64           `json:"diskTotal"`
	DiskUsed       uint64           `json:"diskUsed"`
	GPUs           []ComputeGPU     `json:"gpus"`
	Nics           []ComputeNic     `json:"nics"`
	Rdma           []ComputeRdma    `json:"rdma"`
	Processes      []ComputeProcess `json:"processes"`
	Warnings       []string         `json:"warnings"`
	CollectMs      int64            `json:"collectMs"`
}

type ComputeHostView struct {
	model.ComputeHost
	// SourceName / SourceDetail describe where the host is managed
	// (terminal host name + user@addr:port, or node name + entry URL).
	SourceName    string              `json:"sourceName"`
	SourceDetail  string              `json:"sourceDetail"`
	SourceMissing bool                `json:"sourceMissing"`
	Realtime      ComputeHostRealtime `json:"realtime"`
	Domain        string              `json:"domain"`
	InstanceNames []string            `json:"instanceNames"`
}

// ---- topology ------------------------------------------------------------

type ComputeLinkReq struct {
	HostAID uint   `json:"hostAId" validate:"required"`
	HostBID uint   `json:"hostBId" validate:"required"`
	IfaceA  string `json:"ifaceA"`
	IfaceB  string `json:"ifaceB"`
	IPA     string `json:"ipA"`
	IPB     string `json:"ipB"`
}

type ComputeFabricDomain struct {
	Name    string                    `json:"name"`
	HostIDs []uint                    `json:"hostIds"`
	Links   []model.ComputeFabricLink `json:"links"`
	Healthy bool                      `json:"healthy"`
}

// ---- templates / instances ------------------------------------------------

type ComputeRuntimeReq struct {
	ID             uint   `json:"id"`
	Name           string `json:"name" validate:"required,max=64"`
	Engine         string `json:"engine" validate:"required,oneof=vllm sglang other"`
	Device         string `json:"device" validate:"omitempty,oneof=gpu npu"`
	Backend        string `json:"backend" validate:"omitempty,oneof=script vllm"`
	Image          string `json:"image"`
	MultiNode      bool   `json:"multiNode"`
	LaunchTemplate string `json:"launchTemplate"`
	StopTemplate   string `json:"stopTemplate"`
	HealthPath     string `json:"healthPath"`
	MetricsPath    string `json:"metricsPath"`
	Remark         string `json:"remark"`
}

type ComputeProfileReq struct {
	ID              uint   `json:"id"`
	Name            string `json:"name" validate:"required,max=64"`
	RuntimeID       uint   `json:"runtimeId" validate:"required"`
	ServedName      string `json:"servedName" validate:"required,max=128"`
	ModelPath       string `json:"modelPath" validate:"required"`
	NodeCount       int    `json:"nodeCount" validate:"min=1,max=64"`
	NeedFabric      bool   `json:"needFabric"`
	MinMemGB        int    `json:"minMemGb"`
	Port            int    `json:"port" validate:"omitempty,min=1,max=65535"`
	ExtraArgs       string `json:"extraArgs"`
	Env             string `json:"env"`
	StartTimeoutSec int    `json:"startTimeoutSec"`
	Remark          string `json:"remark"`
}

type ComputeEngineMetrics struct {
	Healthy          bool    `json:"healthy"`
	Running          float64 `json:"running"`
	Waiting          float64 `json:"waiting"`
	KVUsage          float64 `json:"kvUsage"`
	TTFTMs           float64 `json:"ttftMs"`  // average over the last interval
	QueueMs          float64 `json:"queueMs"` // average over the last interval
	PromptTPS        float64 `json:"promptTps"`
	GenTPS           float64 `json:"genTps"`
	Inflight         int64   `json:"inflight"` // requests in flight through the router
	LastActiveAt     int64   `json:"lastActiveAt"`
	SampledAt        int64   `json:"sampledAt"`
	Error            string  `json:"error"`
	ReportedModel    string  `json:"reportedModel"`
	GPUMemUtil       float64 `json:"gpuMemUtil"`
	PromptTokens     float64 `json:"promptTokens"`
	GenerationTokens float64 `json:"generationTokens"`
}

type ComputeInstanceView struct {
	model.ComputeInstance
	HostNames   []string             `json:"hostNames"`
	ProfileName string               `json:"profileName"`
	Metrics     ComputeEngineMetrics `json:"metrics"`
}

type ComputeInstanceReq struct {
	ID            uint   `json:"id"`
	Name          string `json:"name" validate:"required,max=64"`
	ServedName    string `json:"servedName" validate:"required"`
	UpstreamModel string `json:"upstreamModel"`
	HeadHostID    uint   `json:"headHostId" validate:"required"`
	HostIDs       []uint `json:"hostIds"`
	Port          int    `json:"port" validate:"required,min=1,max=65535"`
	APIKey        string `json:"apiKey"`
	ContainerName string `json:"containerName"`
	Weight        int    `json:"weight"`
}

type ComputeAdoptReq struct {
	HostID     uint   `json:"hostId" validate:"required"`
	Port       int    `json:"port" validate:"required"`
	Name       string `json:"name"`
	ServedName string `json:"servedName"`
	APIKey     string `json:"apiKey"`
	// Extra hosts of a multi-host group (workers); head is HostID.
	WorkerHostIDs []uint `json:"workerHostIds"`
}

type ComputeRouteReq struct {
	ID           uint `json:"id" validate:"required"`
	RouteEnabled bool `json:"routeEnabled"`
	Weight       int  `json:"weight" validate:"min=0,max=1000"`
}

// ---- jobs ----------------------------------------------------------------

type ComputeDeployReq struct {
	ProfileID uint   `json:"profileId" validate:"required"`
	HostIDs   []uint `json:"hostIds"` // empty = let the scheduler choose
	Name      string `json:"name"`
	DryRun    bool   `json:"dryRun"`
}

type ComputeSwitchReq struct {
	InstanceID uint `json:"instanceId" validate:"required"`
	ProfileID  uint `json:"profileId" validate:"required"`
	DryRun     bool `json:"dryRun"`
}

type ComputeInstanceOpReq struct {
	ID        uint   `json:"id" validate:"required"`
	Operation string `json:"operation" validate:"required,oneof=stop start restart remove drain undrain"`
	// Adopted instances are only touched with Force (the panel did not
	// start them and may not know how to bring them back).
	Force bool `json:"force"`
}

type ComputeJobStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // pending / running / success / failed / skipped
	Message    string `json:"message"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
}

type ComputePrecheckItem struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type ComputePrecheckRes struct {
	OK      bool                  `json:"ok"`
	HostIDs []uint                `json:"hostIds"`
	Items   []ComputePrecheckItem `json:"items"`
	JobID   uint                  `json:"jobId"`
}

// ---- router / consumers ---------------------------------------------------

type ComputeConsumerReq struct {
	ID            uint   `json:"id"`
	Name          string `json:"name" validate:"required,max=64"`
	AllowedModels string `json:"allowedModels"`
	RPM           int    `json:"rpm" validate:"min=0"`
	Enabled       *bool  `json:"enabled"`
	Remark        string `json:"remark"`
}

type ComputeConsumerKeyRes struct {
	ID  uint   `json:"id"`
	Key string `json:"key"`
}

type ComputeRouterSetting struct {
	Enabled        bool   `json:"enabled"`
	Listen         string `json:"listen" validate:"required"`
	MaxRetries     int    `json:"maxRetries" validate:"min=0,max=3"`
	RequestTimeout int    `json:"requestTimeout" validate:"min=10,max=3600"` // seconds
	Running        bool   `json:"running"`
	Error          string `json:"error"`
	// Scheduler switches live here as well (one "settings" form).
	SchedulerPaused bool `json:"schedulerPaused"`
	WatchFast       bool `json:"watchFast"`
}

// ---- scheduler ------------------------------------------------------------

type ComputePolicyReq struct {
	ID            uint    `json:"id"`
	ServedName    string  `json:"servedName" validate:"required"`
	ProfileID     uint    `json:"profileId"`
	Mode          string  `json:"mode" validate:"required,oneof=off suggest auto"`
	MinInstances  int     `json:"minInstances" validate:"min=0"`
	MaxInstances  int     `json:"maxInstances" validate:"min=0"`
	HotSpare      int     `json:"hotSpare" validate:"min=0"`
	Priority      int     `json:"priority" validate:"min=0,max=100"`
	Preemptible   bool    `json:"preemptible"`
	QueueHigh     float64 `json:"queueHigh"`
	KVHigh        float64 `json:"kvHigh"`
	TTFTHigh      float64 `json:"ttftHigh" validate:"min=0"`
	SustainRounds int     `json:"sustainRounds"`
	IdleMinutes   int     `json:"idleMinutes"`
	CooldownSec   int     `json:"cooldownSec"`
}

// ComputePoolReq creates / updates a compute pool: a policy plus (on
// create) a router key and an AI → Model Services entry named Name.
type ComputePoolReq struct {
	ComputePolicyReq
	Name string `json:"name" validate:"required,max=64"`
}

type ComputePoolDeleteReq struct {
	ID            uint `json:"id" validate:"required"`
	RemoveService bool `json:"removeService"`
}

type ComputePoolRes struct {
	ID        uint   `json:"id"`
	AccountID uint   `json:"accountId"`
	Key       string `json:"key"` // router key, only returned when created
}

type ComputePoolView struct {
	model.ComputePolicy
	ProfileName string `json:"profileName"`
	NodeCount   int    `json:"nodeCount"`
	Instances   int    `json:"instances"`
	Healthy     int    `json:"healthy"`
	// ServiceMissing: AccountID is set but the model service was deleted.
	ServiceMissing bool   `json:"serviceMissing"`
	BaseURL        string `json:"baseUrl"`
}

type ComputeSuggestion struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"` // scaleUp / scaleDown / preempt / minViolation / hotSpare / fabric
	ServedName string `json:"servedName"`
	ProfileID  uint   `json:"profileId"`
	InstanceID uint   `json:"instanceId"`
	HostIDs    []uint `json:"hostIds"`
	Reason     string `json:"reason"`
	Severity   string `json:"severity"` // info / warning / critical
	CreatedAt  int64  `json:"createdAt"`
	Actionable bool   `json:"actionable"`
}

type ComputeSuggestionOpReq struct {
	ID        string `json:"id" validate:"required"`
	Operation string `json:"operation" validate:"required,oneof=apply dismiss"`
}

// ---- overview ------------------------------------------------------------

type ComputeModelSummary struct {
	ServedName   string  `json:"servedName"`
	Instances    int     `json:"instances"`
	Healthy      int     `json:"healthy"`
	Running      float64 `json:"running"`
	Waiting      float64 `json:"waiting"`
	KVUsage      float64 `json:"kvUsage"`
	TTFTMs       float64 `json:"ttftMs"`
	GenTPS       float64 `json:"genTps"`
	PolicyMode   string  `json:"policyMode"`
	MinInstances int     `json:"minInstances"`
	MaxInstances int     `json:"maxInstances"`
}

type ComputeOverview struct {
	Hosts         int                   `json:"hosts"`
	HostsOnline   int                   `json:"hostsOnline"`
	HostsIdle     int                   `json:"hostsIdle"`
	HostsCordoned int                   `json:"hostsCordoned"`
	GPUs          int                   `json:"gpus"`
	NPUs          int                   `json:"npus"`
	GPUUtilAvg    float64               `json:"gpuUtilAvg"`
	MemTotal      uint64                `json:"memTotal"`
	MemUsed       uint64                `json:"memUsed"`
	Instances     int                   `json:"instances"`
	InstHealthy   int                   `json:"instHealthy"`
	Domains       int                   `json:"domains"`
	FabricIssues  int                   `json:"fabricIssues"`
	RunningJobs   int                   `json:"runningJobs"`
	Suggestions   int                   `json:"suggestions"`
	GenTPS        float64               `json:"genTps"`
	Models        []ComputeModelSummary `json:"models"`
	Warnings      []string              `json:"warnings"`
	Router        ComputeRouterSetting  `json:"router"`
}

type ComputeWatchReq struct {
	Fast bool `json:"fast"`
}

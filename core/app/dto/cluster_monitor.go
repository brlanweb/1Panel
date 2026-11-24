package dto

// ClusterMonitorRealtime is one node's high-frequency sample (CPU and
// network rate). The master polls each node at most once per second and
// only while somebody has the monitor page open.
type ClusterMonitorRealtime struct {
	NodeID         uint    `json:"nodeId"`
	Name           string  `json:"name"`
	Online         bool    `json:"online"`
	CpuUsedPercent float64 `json:"cpuUsedPercent"`
	CpuTotal       int     `json:"cpuTotal"`
	Load1          float64 `json:"load1"`
	// Bytes per second, computed from two consecutive samples.
	UpSpeed   float64 `json:"upSpeed"`
	DownSpeed float64 `json:"downSpeed"`
	// Cumulative traffic counters since boot.
	NetBytesSent uint64 `json:"netBytesSent"`
	NetBytesRecv uint64 `json:"netBytesRecv"`
	Error        string `json:"error"`
	SampledAt    int64  `json:"sampledAt"`
}

type ClusterMonitorDisk struct {
	Path        string  `json:"path"`
	Type        string  `json:"type"`
	Device      string  `json:"device"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"usedPercent"`
}

// ClusterMonitorStatic is one node's slow-changing snapshot (memory, disks,
// addresses), refreshed every ~30 seconds and served from cache otherwise.
type ClusterMonitorStatic struct {
	NodeID       uint   `json:"nodeId"`
	Name         string `json:"name"`
	EntryHost    string `json:"entryHost"`
	Online       bool   `json:"online"`
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	PrettyDistro string `json:"prettyDistro"`
	KernelArch   string `json:"kernelArch"`
	Uptime       uint64 `json:"uptime"`
	CpuTotal     int    `json:"cpuTotal"`
	CpuModelName string `json:"cpuModelName"`

	MemoryTotal       uint64  `json:"memoryTotal"`
	MemoryUsed        uint64  `json:"memoryUsed"`
	MemoryUsedPercent float64 `json:"memoryUsedPercent"`
	SwapTotal         uint64  `json:"swapTotal"`
	SwapUsed          uint64  `json:"swapUsed"`
	SwapUsedPercent   float64 `json:"swapUsedPercent"`

	DiskTotal       uint64               `json:"diskTotal"`
	DiskUsed        uint64               `json:"diskUsed"`
	DiskUsedPercent float64              `json:"diskUsedPercent"`
	Disks           []ClusterMonitorDisk `json:"disks"`

	InternalIPs []string `json:"internalIPs"`
	PublicIP    string   `json:"publicIP"`

	// CpuTemperature is the hottest CPU sensor in °C (0 = not available).
	CpuTemperature float64 `json:"cpuTemperature"`
	// GPUs is empty on nodes without accelerators. GpuUtilMax /
	// GpuTempMax summarise the busiest / hottest device.
	GPUs       []ClusterMonitorGPU `json:"gpus"`
	GpuUtilMax float64             `json:"gpuUtilMax"`
	GpuTempMax float64             `json:"gpuTempMax"`

	Error     string `json:"error"`
	SampledAt int64  `json:"sampledAt"`
}

// ClusterMonitorGPU is one accelerator of a node. Nil metrics were not
// reported by the vendor tool.
type ClusterMonitorGPU struct {
	Index       int      `json:"index"`
	Kind        string   `json:"kind"`
	Vendor      string   `json:"vendor"`
	Name        string   `json:"name"`
	Utilization *float64 `json:"utilization"`
	Temperature *float64 `json:"temperature"`
	MemoryUsed  *float64 `json:"memoryUsed"`
	MemoryTotal *float64 `json:"memoryTotal"`
}

// ClusterMonitorRisk is one threshold breach found in the latest samples.
type ClusterMonitorRisk struct {
	NodeID  uint    `json:"nodeId"`
	Name    string  `json:"name"`
	Kind    string  `json:"kind"`  // offline / cpu / memory / disk / cpuTemp / gpuTemp
	Level   string  `json:"level"` // warning / critical
	Value   float64 `json:"value"`
	Target  string  `json:"target"` // disk mount path for disk risks
	Message string  `json:"message"`
}

type ClusterMonitorStaticRes struct {
	Nodes    []ClusterMonitorStatic `json:"nodes"`
	Risks    []ClusterMonitorRisk   `json:"risks"`
	Total    int                    `json:"total"`
	Online   int                    `json:"online"`
	Offline  int                    `json:"offline"`
	Warning  int                    `json:"warning"`
	Critical int                    `json:"critical"`
	// Thresholds used to compute the risks, so the UI can colour bars
	// consistently with the risk list.
	WarnPercent     float64 `json:"warnPercent"`
	CriticalPercent float64 `json:"criticalPercent"`
	// Temperature thresholds (°C) for CPU / GPU temperature risks.
	WarnTemp     float64 `json:"warnTemp"`
	CriticalTemp float64 `json:"criticalTemp"`
}

type ClusterMonitorRealtimeRes struct {
	Nodes []ClusterMonitorRealtime `json:"nodes"`
}

type ClusterMonitorStaticReq struct {
	Force bool `json:"force"`
	// WithHW also collects CPU temperature / GPU metrics on the nodes.
	WithHW bool `json:"withHW"`
}

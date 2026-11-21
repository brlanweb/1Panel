package dto

// MonitorRealtime is the cheap, high-frequency sample used by the cluster
// resource monitor (polled about once per second while the page is open).
// It never shells out (no df / no accelerator probes), so it is safe to call
// at 1 Hz on every node.
type MonitorRealtime struct {
	CPUUsedPercent float64 `json:"cpuUsedPercent"`
	CPUTotal       int     `json:"cpuTotal"`
	Load1          float64 `json:"load1"`
	// Cumulative byte counters over the physical interfaces (loopback,
	// docker bridges and veth pairs excluded so container traffic is not
	// counted twice). Rates are computed by the caller from two samples.
	NetBytesSent uint64 `json:"netBytesSent"`
	NetBytesRecv uint64 `json:"netBytesRecv"`
	// ShotTime is the sample time in unix milliseconds.
	ShotTime int64 `json:"shotTime"`
}

// MonitorStatic is the slow-changing part of a node snapshot (memory,
// disks, addresses). The cluster monitor refreshes it every ~30 seconds.
type MonitorStatic struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	PrettyDistro string `json:"prettyDistro"`
	KernelArch   string `json:"kernelArch"`
	Uptime       uint64 `json:"uptime"`

	CPUTotal     int    `json:"cpuTotal"`
	CPUModelName string `json:"cpuModelName"`

	MemoryTotal       uint64  `json:"memoryTotal"`
	MemoryUsed        uint64  `json:"memoryUsed"`
	MemoryUsedPercent float64 `json:"memoryUsedPercent"`
	SwapTotal         uint64  `json:"swapTotal"`
	SwapUsed          uint64  `json:"swapUsed"`
	SwapUsedPercent   float64 `json:"swapUsedPercent"`

	Disks []DiskInfo `json:"disks"`

	InternalIPs []string `json:"internalIPs"`
	PublicIP    string   `json:"publicIP"`

	NetBytesSent uint64 `json:"netBytesSent"`
	NetBytesRecv uint64 `json:"netBytesRecv"`
}

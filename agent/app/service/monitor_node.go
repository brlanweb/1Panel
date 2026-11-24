package service

import (
	"context"
	"io"
	network "net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/utils/psutil"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// Cluster resource monitor samples. Split in two so the master can poll
// them at different rates:
//
//   - realtime: CPU + network counters, no shell-outs, ~1 Hz while the
//     monitor page is open (and never otherwise — nothing here runs in the
//     background);
//   - static: memory, disks, IP addresses, ~every 30 s.

// virtualIfacePrefixes are interfaces whose traffic is either loopback or a
// copy of traffic already counted on a physical NIC (docker bridge, veth
// pairs, k8s overlay), so they are excluded from the totals.
var virtualIfacePrefixes = []string{"lo", "docker", "br-", "veth", "virbr", "cni", "flannel", "cali", "kube", "vxlan", "tunl", "dummy"}

func isVirtualIface(name string) bool {
	for _, prefix := range virtualIfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func loadPhysicalNetCounters() (sent, recv uint64) {
	counters, err := net.IOCounters(true)
	if err != nil {
		return 0, 0
	}
	for _, item := range counters {
		if isVirtualIface(item.Name) {
			continue
		}
		sent += item.BytesSent
		recv += item.BytesRecv
	}
	return sent, recv
}

func (u *DashboardService) LoadMonitorRealtime() *dto.MonitorRealtime {
	var data dto.MonitorRealtime
	used, perCore, _ := psutil.CPU.GetCPUUsage()
	data.CPUUsedPercent = used
	data.CPUTotal = len(perCore)
	if data.CPUTotal == 0 {
		data.CPUTotal = psutil.CPU.NumCPU()
	}
	if avg, err := load.Avg(); err == nil {
		data.Load1 = avg.Load1
	}
	data.NetBytesSent, data.NetBytesRecv = loadPhysicalNetCounters()
	data.ShotTime = time.Now().UnixMilli()
	return &data
}

// LoadMonitorStatic returns the static sample. withHW adds CPU temperature
// and accelerator metrics; they shell out to vendor tools (nvidia-smi ...),
// so they are only collected when a user is looking at a page that shows
// them, never for the background alert sampler.
func (u *DashboardService) LoadMonitorStatic(withHW bool) *dto.MonitorStatic {
	var data dto.MonitorStatic
	if hostInfo, err := psutil.HOST.GetHostInfo(false); err == nil {
		data.Hostname = hostInfo.Hostname
		data.OS = hostInfo.OS
		data.KernelArch = hostInfo.KernelArch
		data.Uptime = hostInfo.Uptime
		if hostInfo.BootTime > 0 {
			if now := uint64(time.Now().Unix()); now > hostInfo.BootTime {
				data.Uptime = now - hostInfo.BootTime
			}
		}
	}
	data.PrettyDistro = psutil.HOST.GetDistro()
	data.CPUTotal, _ = psutil.CPUInfo.GetLogicalCores(false)
	if cpuInfo, err := psutil.CPUInfo.GetCPUInfo(false); err == nil && len(cpuInfo) > 0 {
		data.CPUModelName = cpuInfo[0].ModelName
	}
	if memInfo, err := mem.VirtualMemory(); err == nil {
		data.MemoryTotal = memInfo.Total
		data.MemoryUsed = memInfo.Used
		data.MemoryUsedPercent = memInfo.UsedPercent
	}
	if swapInfo, err := mem.SwapMemory(); err == nil {
		data.SwapTotal = swapInfo.Total
		data.SwapUsed = swapInfo.Used
		data.SwapUsedPercent = swapInfo.UsedPercent
	}
	data.Disks = loadDiskInfo(false)
	data.InternalIPs = loadInternalIPs()
	data.PublicIP = publicIPCache.get()
	data.NetBytesSent, data.NetBytesRecv = loadPhysicalNetCounters()
	if withHW {
		hw := loadHardwareSample()
		data.CPUTemperature = hw.cpuTemp
		data.GPUs = hw.gpus
		data.HWSampled = true
	}
	return &data
}

// loadInternalIPs returns the IPv4 addresses of the physical interfaces,
// with the default-route address first.
func loadInternalIPs() []string {
	var result []string
	seen := map[string]bool{}
	if outbound := loadOutboundIP(); outbound != "" && outbound != "IPNotFound" {
		result = append(result, outbound)
		seen[outbound] = true
	}
	ifaces, err := network.Interfaces()
	if err != nil {
		return result
	}
	for _, iface := range ifaces {
		if iface.Flags&network.FlagUp == 0 || iface.Flags&network.FlagLoopback != 0 || isVirtualIface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*network.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			ip := ipNet.IP.String()
			if seen[ip] {
				continue
			}
			seen[ip] = true
			result = append(result, ip)
		}
	}
	return result
}

// publicIPCache resolves the node's public IPv4 through a few plain-text
// echo services. The lookup never blocks a request: a stale value triggers
// a background refresh and the previous value (possibly empty on the very
// first call) is returned immediately.
var publicIPCache = &publicIPResolver{}

type publicIPResolver struct {
	mu        sync.Mutex
	value     string
	expiresAt time.Time
	running   bool
}

var publicIPEndpoints = []string{
	"https://api.ipify.org",
	"https://ipv4.icanhazip.com",
	"https://ifconfig.me/ip",
}

func (p *publicIPResolver) get() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Now().After(p.expiresAt) && !p.running {
		p.running = true
		go p.refresh()
	}
	return p.value
}

func (p *publicIPResolver) refresh() {
	ip := lookupPublicIP()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = false
	if ip != "" {
		p.value = ip
		p.expiresAt = time.Now().Add(30 * time.Minute)
		return
	}
	// Keep the last known value, retry sooner.
	p.expiresAt = time.Now().Add(5 * time.Minute)
}

func lookupPublicIP() string {
	client := &http.Client{Timeout: 4 * time.Second}
	for _, endpoint := range publicIPEndpoints {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("User-Agent", "curl/8.0")
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		_ = resp.Body.Close()
		cancel()
		ip := strings.TrimSpace(string(body))
		if resp.StatusCode == http.StatusOK && network.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

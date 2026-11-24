package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

// Cluster resource monitor.
//
// Two independent pipelines with different refresh rates, both pull-based
// and driven only by the monitor page (nothing polls in the background, so
// closing the page stops all monitoring traffic):
//
//   - realtime (CPU, network speed, traffic): the page asks every second;
//     results are cached for realtimeTTL so several open tabs / users share
//     one fan-out per second, and concurrent requests are collapsed with
//     singleflight.
//   - static (memory, disks, IP addresses, uptime): cached for staticTTL
//     (30s); the page asks every 30s and gets the cached copy in between.
//
// A node that fails is backed off for a few seconds so an unreachable host
// does not cost a full timeout on every 1s tick.
const (
	realtimeTTL          = 800 * time.Millisecond
	realtimeTimeout      = 3 * time.Second
	staticTTL            = 30 * time.Second
	staticForceMinAge    = 5 * time.Second
	staticTimeout        = 10 * time.Second
	monitorFailBackoff   = 10 * time.Second
	monitorWarnPercent   = 80.0
	monitorCritPercent   = 90.0
	monitorWarnTemp      = 80.0
	monitorCritTemp      = 90.0
	monitorFanoutLimit   = 8
	monitorLocalNodeName = "local"
)

type ClusterMonitorService struct {
	group singleflight.Group

	mu         sync.Mutex
	realtime   dto.ClusterMonitorRealtimeRes
	realtimeAt time.Time
	static     dto.ClusterMonitorStaticRes
	staticAt   time.Time
	// staticHW is true when the cached static sample includes hardware
	// sensors (CPU temperature / GPU).
	staticHW     bool
	prevCounters map[uint]netSample
	failedUntil  map[string]time.Time
	// legacyNodes are remote panels without /dashboard/monitor/* (older
	// agents); they are read through the regular dashboard endpoints.
	legacyNodes map[uint]bool
	lastStatic  map[uint]dto.ClusterMonitorStatic
}

type netSample struct {
	sent, recv uint64
	at         int64 // unix ms
}

type IClusterMonitorService interface {
	Realtime() (dto.ClusterMonitorRealtimeRes, error)
	// Static returns the cached static sample. withHW asks the nodes for
	// CPU temperature / GPU metrics too (vendor tool shell-outs); only
	// pages a user is looking at pass true, the alert sampler never does.
	Static(force, withHW bool) (dto.ClusterMonitorStaticRes, error)
}

// sharedClusterMonitor is the single cache used by the monitor page and the
// background alert sampler, so both share one fan-out to the nodes.
var sharedClusterMonitor = &ClusterMonitorService{
	prevCounters: map[uint]netSample{},
	failedUntil:  map[string]time.Time{},
	legacyNodes:  map[uint]bool{},
	lastStatic:   map[uint]dto.ClusterMonitorStatic{},
}

func NewIClusterMonitorService() IClusterMonitorService {
	return sharedClusterMonitor
}

// monitorNodes returns the local panel (ID 0) followed by every registered
// cluster node.
func monitorNodes() ([]model.ClusterNode, error) {
	nodes, err := clusterNodeRepo.GetList()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return append([]model.ClusterNode{{ID: 0, Name: monitorLocalNodeName, Status: model.ClusterNodeStatusOnline}}, nodes...), nil
}

func (s *ClusterMonitorService) callNode(node model.ClusterNode, path string, out interface{}, timeout time.Duration) error {
	if node.ID == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return (&ClusterSyncService{}).callLocalJSON(ctx, http.MethodGet, path, nil, out, timeout)
	}
	return (&ClusterNodeService{}).callRemoteJSON(node, http.MethodGet, path, nil, out, timeout)
}

func (s *ClusterMonitorService) inBackoff(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.failedUntil[key])
}

func (s *ClusterMonitorService) markFailed(key string, failed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if failed {
		s.failedUntil[key] = time.Now().Add(monitorFailBackoff)
	} else {
		delete(s.failedUntil, key)
	}
}

func (s *ClusterMonitorService) isLegacy(id uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.legacyNodes[id]
}

func (s *ClusterMonitorService) setLegacy(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.legacyNodes[id] = true
}

// ------------------------------------------------------------- realtime

func (s *ClusterMonitorService) Realtime() (dto.ClusterMonitorRealtimeRes, error) {
	s.mu.Lock()
	if time.Since(s.realtimeAt) < realtimeTTL {
		cached := s.realtime
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()
	res, err, _ := s.group.Do("realtime", func() (interface{}, error) {
		return s.refreshRealtime()
	})
	if err != nil {
		return dto.ClusterMonitorRealtimeRes{}, err
	}
	return res.(dto.ClusterMonitorRealtimeRes), nil
}

type agentRealtime struct {
	CPUUsedPercent float64 `json:"cpuUsedPercent"`
	CPUTotal       int     `json:"cpuTotal"`
	Load1          float64 `json:"load1"`
	NetBytesSent   uint64  `json:"netBytesSent"`
	NetBytesRecv   uint64  `json:"netBytesRecv"`
	ShotTime       int64   `json:"shotTime"`
}

func (s *ClusterMonitorService) refreshRealtime() (dto.ClusterMonitorRealtimeRes, error) {
	nodes, err := monitorNodes()
	if err != nil {
		return dto.ClusterMonitorRealtimeRes{}, err
	}
	results := make([]dto.ClusterMonitorRealtime, len(nodes))
	group, _ := errgroup.WithContext(context.Background())
	group.SetLimit(monitorFanoutLimit)
	for i, node := range nodes {
		i, node := i, node
		group.Go(func() error {
			results[i] = s.sampleRealtime(node)
			return nil
		})
	}
	_ = group.Wait()
	res := dto.ClusterMonitorRealtimeRes{Nodes: results}
	s.mu.Lock()
	s.realtime = res
	s.realtimeAt = time.Now()
	s.mu.Unlock()
	return res, nil
}

func (s *ClusterMonitorService) sampleRealtime(node model.ClusterNode) dto.ClusterMonitorRealtime {
	item := dto.ClusterMonitorRealtime{NodeID: node.ID, Name: node.Name, SampledAt: time.Now().UnixMilli()}
	key := fmt.Sprintf("rt-%d", node.ID)
	// A stored "offline" is only a hint from the last probe. Skipping the
	// call here is what kept a recovered node offline until someone opened
	// the node page and clicked check. The failure backoff below already
	// stops an unreachable host from timing out on every tick.
	if s.inBackoff(key) {
		item.Error = "node unreachable, retrying shortly"
		return item
	}
	var data agentRealtime
	var err error
	if node.ID != 0 && s.isLegacy(node.ID) {
		err = s.legacyRealtime(node, &data)
	} else {
		err = s.callNode(node, "/api/v2/dashboard/monitor/realtime", &data, realtimeTimeout)
		if err != nil && node.ID != 0 && isMissingRouteErr(err) {
			s.setLegacy(node.ID)
			err = s.legacyRealtime(node, &data)
		}
	}
	s.markFailed(key, err != nil)
	if err != nil {
		item.Error = truncateErr(err.Error())
		return item
	}
	item.Online = true
	item.CpuUsedPercent = data.CPUUsedPercent
	item.CpuTotal = data.CPUTotal
	item.Load1 = data.Load1
	item.NetBytesSent = data.NetBytesSent
	item.NetBytesRecv = data.NetBytesRecv
	if data.ShotTime == 0 {
		data.ShotTime = time.Now().UnixMilli()
	}
	item.SampledAt = data.ShotTime

	s.mu.Lock()
	prev, ok := s.prevCounters[node.ID]
	s.prevCounters[node.ID] = netSample{sent: data.NetBytesSent, recv: data.NetBytesRecv, at: data.ShotTime}
	s.mu.Unlock()
	if ok && data.ShotTime > prev.at && data.NetBytesSent >= prev.sent && data.NetBytesRecv >= prev.recv {
		seconds := float64(data.ShotTime-prev.at) / 1000
		// Ignore stale previous samples (page was closed for a while):
		// an average over minutes is not a "current speed".
		if seconds > 0 && seconds <= 30 {
			item.UpSpeed = float64(data.NetBytesSent-prev.sent) / seconds
			item.DownSpeed = float64(data.NetBytesRecv-prev.recv) / seconds
		}
	}
	return item
}

// legacyRealtime reads an older remote agent: CPU from /current/node (cheap)
// and the network counters from /current/all/all.
func (s *ClusterMonitorService) legacyRealtime(node model.ClusterNode, out *agentRealtime) error {
	var current struct {
		CPUUsedPercent float64 `json:"cpuUsedPercent"`
		CPUTotal       int     `json:"cpuTotal"`
		Load1          float64 `json:"load1"`
		NetBytesSent   uint64  `json:"netBytesSent"`
		NetBytesRecv   uint64  `json:"netBytesRecv"`
	}
	if err := s.callNode(node, "/api/v2/dashboard/current/all/all", &current, realtimeTimeout+2*time.Second); err != nil {
		return err
	}
	out.CPUUsedPercent, out.CPUTotal, out.Load1 = current.CPUUsedPercent, current.CPUTotal, current.Load1
	out.NetBytesSent, out.NetBytesRecv = current.NetBytesSent, current.NetBytesRecv
	out.ShotTime = time.Now().UnixMilli()
	return nil
}

// --------------------------------------------------------------- static

func (s *ClusterMonitorService) Static(force, withHW bool) (dto.ClusterMonitorStaticRes, error) {
	s.mu.Lock()
	age := time.Since(s.staticAt)
	// A cached sample with hardware serves both kinds of callers; one
	// without hardware only serves callers that did not ask for it.
	if age < staticTTL && (!force || age < staticForceMinAge) && (s.staticHW || !withHW) {
		cached := s.static
		s.mu.Unlock()
		return s.withRisks(cached), nil
	}
	s.mu.Unlock()
	key := "static"
	if withHW {
		key = "static-hw"
	}
	res, err, _ := s.group.Do(key, func() (interface{}, error) {
		return s.refreshStatic(withHW)
	})
	if err != nil {
		return dto.ClusterMonitorStaticRes{}, err
	}
	return s.withRisks(res.(dto.ClusterMonitorStaticRes)), nil
}

type agentStatic struct {
	Hostname          string                   `json:"hostname"`
	OS                string                   `json:"os"`
	PrettyDistro      string                   `json:"prettyDistro"`
	KernelArch        string                   `json:"kernelArch"`
	Uptime            uint64                   `json:"uptime"`
	CPUTotal          int                      `json:"cpuTotal"`
	CPUModelName      string                   `json:"cpuModelName"`
	MemoryTotal       uint64                   `json:"memoryTotal"`
	MemoryUsed        uint64                   `json:"memoryUsed"`
	MemoryUsedPercent float64                  `json:"memoryUsedPercent"`
	SwapTotal         uint64                   `json:"swapTotal"`
	SwapUsed          uint64                   `json:"swapUsed"`
	SwapUsedPercent   float64                  `json:"swapUsedPercent"`
	Disks             []dto.ClusterMonitorDisk `json:"disks"`
	InternalIPs       []string                 `json:"internalIPs"`
	PublicIP          string                   `json:"publicIP"`
	CPUTemperature    float64                  `json:"cpuTemperature"`
	GPUs              []dto.ClusterMonitorGPU  `json:"gpus"`
	HWSampled         bool                     `json:"hwSampled"`
}

func (s *ClusterMonitorService) refreshStatic(withHW bool) (dto.ClusterMonitorStaticRes, error) {
	nodes, err := monitorNodes()
	if err != nil {
		return dto.ClusterMonitorStaticRes{}, err
	}
	results := make([]dto.ClusterMonitorStatic, len(nodes))
	group, _ := errgroup.WithContext(context.Background())
	group.SetLimit(monitorFanoutLimit)
	for i, node := range nodes {
		i, node := i, node
		group.Go(func() error {
			results[i] = s.sampleStatic(node, withHW)
			return nil
		})
	}
	_ = group.Wait()
	res := dto.ClusterMonitorStaticRes{Nodes: results}
	s.mu.Lock()
	s.static = res
	s.staticAt = time.Now()
	s.staticHW = withHW
	s.mu.Unlock()
	return res, nil
}

func (s *ClusterMonitorService) sampleStatic(node model.ClusterNode, withHW bool) dto.ClusterMonitorStatic {
	item := dto.ClusterMonitorStatic{NodeID: node.ID, Name: node.Name, EntryHost: entryHost(node.EntryURL), SampledAt: time.Now().UnixMilli()}
	keepLast := func(errMsg string) dto.ClusterMonitorStatic {
		s.mu.Lock()
		last, ok := s.lastStatic[node.ID]
		s.mu.Unlock()
		if ok {
			// Keep the last known hardware facts (IPs, totals) visible but
			// flag the node as offline.
			last.Online = false
			last.Error = errMsg
			last.Name, last.EntryHost = node.Name, item.EntryHost
			return last
		}
		item.Error = errMsg
		return item
	}
	var data agentStatic
	var err error
	if node.ID != 0 && s.isLegacy(node.ID) {
		err = s.legacyStatic(node, &data)
	} else {
		staticPath := "/api/v2/dashboard/monitor/static"
		if withHW {
			staticPath += "?hw=1"
		}
		err = s.callNode(node, staticPath, &data, staticTimeout)
		if err != nil && node.ID != 0 && isMissingRouteErr(err) {
			s.setLegacy(node.ID)
			err = s.legacyStatic(node, &data)
		}
	}
	if err != nil {
		return keepLast(truncateErr(err.Error()))
	}
	item.Online = true
	item.Hostname, item.OS, item.PrettyDistro, item.KernelArch = data.Hostname, data.OS, data.PrettyDistro, data.KernelArch
	item.Uptime, item.CpuTotal, item.CpuModelName = data.Uptime, data.CPUTotal, data.CPUModelName
	item.MemoryTotal, item.MemoryUsed, item.MemoryUsedPercent = data.MemoryTotal, data.MemoryUsed, data.MemoryUsedPercent
	item.SwapTotal, item.SwapUsed, item.SwapUsedPercent = data.SwapTotal, data.SwapUsed, data.SwapUsedPercent
	item.Disks = data.Disks
	for _, disk := range data.Disks {
		item.DiskTotal += disk.Total
		item.DiskUsed += disk.Used
	}
	if item.DiskTotal > 0 {
		item.DiskUsedPercent = float64(item.DiskUsed) / float64(item.DiskTotal) * 100
	}
	item.InternalIPs = data.InternalIPs
	item.PublicIP = data.PublicIP
	if item.PublicIP == "" && isPublicAddr(item.EntryHost) {
		item.PublicIP = item.EntryHost
	}
	item.CpuTemperature = data.CPUTemperature
	item.GPUs = data.GPUs
	item.GpuUtilMax, item.GpuTempMax = gpuMaxima(data.GPUs)
	s.mu.Lock()
	s.lastStatic[node.ID] = item
	s.mu.Unlock()
	// Only a sample that really read the sensors updates the stored
	// snapshot; a plain (alert sampler) sample must not zero it.
	if node.ID != 0 && withHW && data.HWSampled {
		persistNodeHardware(node.ID, item)
	}
	return item
}

// gpuMaxima returns the highest utilization and temperature across devices.
func gpuMaxima(gpus []dto.ClusterMonitorGPU) (util, temp float64) {
	for _, g := range gpus {
		if g.Utilization != nil && *g.Utilization > util {
			util = *g.Utilization
		}
		if g.Temperature != nil && *g.Temperature > temp {
			temp = *g.Temperature
		}
	}
	return util, temp
}

// persistNodeHardware stores the latest temperature / GPU summary on the
// node row so the node list shows them without opening the monitor page.
func persistNodeHardware(id uint, item dto.ClusterMonitorStatic) {
	_ = clusterNodeRepo.Update(id, map[string]interface{}{
		"cpu_temperature": item.CpuTemperature,
		"gpu_count":       len(item.GPUs),
		"gpu_util_max":    item.GpuUtilMax,
		"gpu_temp_max":    item.GpuTempMax,
	})
}

// legacyStatic reads an older remote agent through /dashboard/base/all/all.
func (s *ClusterMonitorService) legacyStatic(node model.ClusterNode, out *agentStatic) error {
	var base struct {
		Hostname     string `json:"hostname"`
		OS           string `json:"os"`
		PrettyDistro string `json:"prettyDistro"`
		KernelArch   string `json:"kernelArch"`
		IpV4Addr     string `json:"ipV4Addr"`
		CPUModelName string `json:"cpuModelName"`
		CPULogical   int    `json:"cpuLogicalCores"`
		CurrentInfo  struct {
			Uptime                uint64                   `json:"uptime"`
			MemoryTotal           uint64                   `json:"memoryTotal"`
			MemoryUsed            uint64                   `json:"memoryUsed"`
			MemoryUsedPercent     float64                  `json:"memoryUsedPercent"`
			SwapMemoryTotal       uint64                   `json:"swapMemoryTotal"`
			SwapMemoryUsed        uint64                   `json:"swapMemoryUsed"`
			SwapMemoryUsedPercent float64                  `json:"swapMemoryUsedPercent"`
			DiskData              []dto.ClusterMonitorDisk `json:"diskData"`
		} `json:"currentInfo"`
	}
	if err := s.callNode(node, "/api/v2/dashboard/base/all/all", &base, staticTimeout); err != nil {
		return err
	}
	out.Hostname, out.OS, out.PrettyDistro, out.KernelArch = base.Hostname, base.OS, base.PrettyDistro, base.KernelArch
	out.CPUModelName, out.CPUTotal = base.CPUModelName, base.CPULogical
	out.Uptime = base.CurrentInfo.Uptime
	out.MemoryTotal, out.MemoryUsed, out.MemoryUsedPercent = base.CurrentInfo.MemoryTotal, base.CurrentInfo.MemoryUsed, base.CurrentInfo.MemoryUsedPercent
	out.SwapTotal, out.SwapUsed, out.SwapUsedPercent = base.CurrentInfo.SwapMemoryTotal, base.CurrentInfo.SwapMemoryUsed, base.CurrentInfo.SwapMemoryUsedPercent
	out.Disks = base.CurrentInfo.DiskData
	if base.IpV4Addr != "" && base.IpV4Addr != "IPNotFound" {
		out.InternalIPs = []string{base.IpV4Addr}
	}
	return nil
}

// withRisks attaches status counters and threshold breaches computed from
// the static snapshot plus the latest realtime CPU sample.
func (s *ClusterMonitorService) withRisks(res dto.ClusterMonitorStaticRes) dto.ClusterMonitorStaticRes {
	s.mu.Lock()
	cpu := map[uint]float64{}
	for _, item := range s.realtime.Nodes {
		if item.Online {
			cpu[item.NodeID] = item.CpuUsedPercent
		}
	}
	s.mu.Unlock()

	out := dto.ClusterMonitorStaticRes{
		Nodes:           res.Nodes,
		Risks:           []dto.ClusterMonitorRisk{},
		WarnPercent:     monitorWarnPercent,
		CriticalPercent: monitorCritPercent,
		WarnTemp:        monitorWarnTemp,
		CriticalTemp:    monitorCritTemp,
	}
	tempLevel := func(value float64) string {
		switch {
		case value >= monitorCritTemp:
			return "critical"
		case value >= monitorWarnTemp:
			return "warning"
		default:
			return ""
		}
	}
	add := func(risk dto.ClusterMonitorRisk) {
		out.Risks = append(out.Risks, risk)
		if risk.Level == "critical" {
			out.Critical++
		} else {
			out.Warning++
		}
	}
	level := func(value float64) string {
		switch {
		case value >= monitorCritPercent:
			return "critical"
		case value >= monitorWarnPercent:
			return "warning"
		default:
			return ""
		}
	}
	for _, node := range res.Nodes {
		out.Total++
		if !node.Online {
			out.Offline++
			add(dto.ClusterMonitorRisk{NodeID: node.NodeID, Name: node.Name, Kind: "offline", Level: "critical", Message: node.Error})
			continue
		}
		out.Online++
		if value, ok := cpu[node.NodeID]; ok {
			if lv := level(value); lv != "" {
				add(dto.ClusterMonitorRisk{NodeID: node.NodeID, Name: node.Name, Kind: "cpu", Level: lv, Value: value})
			}
		}
		if lv := level(node.MemoryUsedPercent); lv != "" {
			add(dto.ClusterMonitorRisk{NodeID: node.NodeID, Name: node.Name, Kind: "memory", Level: lv, Value: node.MemoryUsedPercent})
		}
		for _, disk := range node.Disks {
			if lv := level(disk.UsedPercent); lv != "" {
				add(dto.ClusterMonitorRisk{NodeID: node.NodeID, Name: node.Name, Kind: "disk", Level: lv, Value: disk.UsedPercent, Target: disk.Path})
			}
		}
		if lv := tempLevel(node.CpuTemperature); lv != "" {
			add(dto.ClusterMonitorRisk{NodeID: node.NodeID, Name: node.Name, Kind: "cpuTemp", Level: lv, Value: node.CpuTemperature})
		}
		// GPU utilization is shown but never raised as a risk: an inference
		// card sitting at 100% is its normal working state. Only the
		// temperature indicates a fault (cooling, fan, airflow).
		for _, gpu := range node.GPUs {
			if gpu.Temperature == nil {
				continue
			}
			if lv := tempLevel(*gpu.Temperature); lv != "" {
				target := fmt.Sprintf("#%d %s", gpu.Index, gpu.Name)
				add(dto.ClusterMonitorRisk{NodeID: node.NodeID, Name: node.Name, Kind: "gpuTemp", Level: lv, Value: *gpu.Temperature, Target: target})
			}
		}
	}
	return out
}

// isMissingRouteErr reports whether a remote call failed because the
// endpoint does not exist on that (older) panel.
func isMissingRouteErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "http 404") || strings.Contains(msg, "unreadable response") ||
		strings.Contains(msg, "not found") || strings.Contains(msg, "code 404")
}

func entryHost(entryURL string) string {
	if entryURL == "" {
		return ""
	}
	parsed, err := url.Parse(entryURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func isPublicAddr(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

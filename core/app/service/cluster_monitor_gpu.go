package service

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
)

// GPU fallback for the cluster monitor.
//
// The static sample asks a node for accelerators through
// /dashboard/monitor/static?hw=1, which only exists on fork builds that
// added hardware sensors. Older nodes, nodes read through the legacy
// dashboard endpoints and nodes whose sensor collection timed out report no
// GPUs there. Every agent (upstream v2.5.1 included) serves the AI -> GPU
// page from /ai/gpu/load, so that endpoint is used to fill the gap.

const (
	gpuFallbackPath    = "/api/v2/ai/gpu/load"
	gpuFallbackTimeout = 20 * time.Second
	// A node that has no accelerators is not asked again for a while: the
	// endpoint also runs the vendor tool / environment probe.
	gpuFallbackNoneTTL = 5 * time.Minute
)

var gpuMetricValue = regexp.MustCompile(`^\s*([+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+))\s*(.*?)\s*$`)

// aiGPULoad is the part of the agent's /ai/gpu/load response (an
// accelerator.Info) the monitor needs. Field names cover both the current
// agent and upstream v2.5.1.
type aiGPULoad struct {
	GPU []struct {
		Index       int    `json:"index"`
		ProductName string `json:"productName"`
		GPUUtil     string `json:"gpuUtil"`
		Temperature string `json:"temperature"`
		MemUsed     string `json:"memUsed"`
		MemTotal    string `json:"memTotal"`
		Type        string `json:"type"`
	} `json:"gpu"`
	NPU []struct {
		Index       int    `json:"index"`
		ProductName string `json:"productName"`
		AICore      string `json:"aiCore"`
		Temperature string `json:"temperature"`
		MemUsed     string `json:"memUsed"`
		MemTotal    string `json:"memTotal"`
		MemoryUsed  string `json:"memoryUsed"`
		MemoryTotal string `json:"memoryTotal"`
		HBMUsed     string `json:"hbmUsed"`
		HBMTotal    string `json:"hbmTotal"`
	} `json:"npu"`
	XPU []struct {
		Basic struct {
			DeviceID   int    `json:"deviceID"`
			DeviceName string `json:"deviceName"`
			Memory     string `json:"memory"`
		} `json:"basic"`
		Stats struct {
			GPUUtil     string `json:"gpuUtil"`
			Temperature string `json:"temperature"`
			MemoryUsed  string `json:"memoryUsed"`
		} `json:"stats"`
	} `json:"xpu"`
}

func (s *ClusterMonitorService) gpuFallbackSkipped(id uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.gpuNoneUntil[id]
	return ok && time.Now().Before(until)
}

func (s *ClusterMonitorService) setGPUFallbackNone(id uint, none bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gpuNoneUntil == nil {
		s.gpuNoneUntil = map[uint]time.Time{}
	}
	if none {
		s.gpuNoneUntil[id] = time.Now().Add(gpuFallbackNoneTTL)
	} else {
		delete(s.gpuNoneUntil, id)
	}
}

// loadGPUFallback reads the node's accelerators through /ai/gpu/load.
// ok is false when the call itself failed (nothing should be cached then).
func (s *ClusterMonitorService) loadGPUFallback(node model.ClusterNode) (gpus []dto.ClusterMonitorGPU, ok bool) {
	if s.gpuFallbackSkipped(node.ID) {
		return nil, true
	}
	var data aiGPULoad
	if err := s.callNode(node, gpuFallbackPath, &data, gpuFallbackTimeout); err != nil {
		return nil, false
	}
	gpus = gpusFromAILoad(data)
	s.setGPUFallbackNone(node.ID, len(gpus) == 0)
	return gpus, true
}

func gpusFromAILoad(data aiGPULoad) []dto.ClusterMonitorGPU {
	var out []dto.ClusterMonitorGPU
	for _, g := range data.GPU {
		vendor := strings.ToLower(strings.TrimSpace(g.Type))
		if vendor == "" {
			vendor = "nvidia"
		}
		out = append(out, dto.ClusterMonitorGPU{
			Index:       g.Index,
			Kind:        "gpu",
			Vendor:      vendor,
			Name:        g.ProductName,
			Utilization: parseGPUMetric(g.GPUUtil, "%"),
			Temperature: parseGPUMetric(g.Temperature, "c"),
			MemoryUsed:  parseGPUMetric(g.MemUsed, "mib"),
			MemoryTotal: parseGPUMetric(g.MemTotal, "mib"),
		})
	}
	for _, n := range data.NPU {
		used, total := parseGPUMetric(n.MemUsed, "mib"), parseGPUMetric(n.MemTotal, "mib")
		if used == nil && total == nil {
			// Older agents only report HBM / DDR separately.
			used, total = parseGPUMetric(n.HBMUsed, "mib"), parseGPUMetric(n.HBMTotal, "mib")
			if used == nil && total == nil {
				used, total = parseGPUMetric(n.MemoryUsed, "mib"), parseGPUMetric(n.MemoryTotal, "mib")
			}
		}
		out = append(out, dto.ClusterMonitorGPU{
			Index:       n.Index,
			Kind:        "npu",
			Vendor:      "ascend",
			Name:        n.ProductName,
			Utilization: parseGPUMetric(n.AICore, "%"),
			Temperature: parseGPUMetric(n.Temperature, "c"),
			MemoryUsed:  used,
			MemoryTotal: total,
		})
	}
	for _, x := range data.XPU {
		out = append(out, dto.ClusterMonitorGPU{
			Index:       x.Basic.DeviceID,
			Kind:        "xpu",
			Vendor:      "intel",
			Name:        x.Basic.DeviceName,
			Utilization: parseGPUMetric(x.Stats.GPUUtil, "%"),
			Temperature: parseGPUMetric(x.Stats.Temperature, "c"),
			MemoryUsed:  parseGPUMetric(x.Stats.MemoryUsed, "mib"),
			MemoryTotal: parseGPUMetric(x.Basic.Memory, "mib"),
		})
	}
	return out
}

// parseGPUMetric turns a vendor tool reading ("45 %", "62 C", "81920 MiB",
// "N/A") into a number in the target unit (%, °C or MiB); nil when the
// value is missing or in an unknown unit.
func parseGPUMetric(raw, target string) *float64 {
	value := strings.TrimSpace(raw)
	if value == "" || strings.EqualFold(value, "N/A") || strings.EqualFold(value, "NA") {
		return nil
	}
	m := gpuMetricValue.FindStringSubmatch(value)
	if len(m) != 3 {
		return nil
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	unit := strings.ToLower(strings.Join(strings.Fields(m[2]), ""))
	var result float64
	switch target {
	case "%":
		if unit != "" && unit != "%" {
			return nil
		}
		if v < 0 || v > 100 {
			return nil
		}
		result = v
	case "c":
		switch unit {
		case "", "c", "°c", "℃":
			result = v
		case "f", "°f", "℉":
			result = (v - 32) * 5 / 9
		default:
			return nil
		}
	case "mib":
		factor, known := map[string]float64{
			"":    1,
			"mib": 1, "mb": 1_000_000.0 / (1024 * 1024),
			"gib": 1024, "gb": 1_000_000_000.0 / (1024 * 1024),
			"kib": 1.0 / 1024, "kb": 1000.0 / (1024 * 1024),
			"b": 1.0 / (1024 * 1024), "byte": 1.0 / (1024 * 1024), "bytes": 1.0 / (1024 * 1024),
			"tib": 1024 * 1024,
		}[unit]
		if !known || v < 0 {
			return nil
		}
		result = v * factor
	default:
		return nil
	}
	return &result
}

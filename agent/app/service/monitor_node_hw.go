package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/utils/ai_tools/accelerator"
	"github.com/shirou/gopsutil/v4/sensors"
)

// Hardware sensors for the cluster monitor static sample: CPU temperature
// and accelerator (GPU / NPU / XPU) utilization + temperature.
//
// Vendor tools (nvidia-smi, amd-smi, npu-smi, xpu-smi) are shell-outs that
// can take seconds, so the result is cached briefly and concurrent callers
// share one collection. Nothing here runs in the background.

const (
	hwSampleTTL     = 10 * time.Second
	hwSampleTimeout = 8 * time.Second
)

type hwSample struct {
	cpuTemp float64
	gpus    []dto.MonitorGPUBrief
}

var hwCache = struct {
	sync.Mutex
	value hwSample
	at    time.Time
	// running collapses concurrent refreshes into one.
	running chan struct{}
}{}

func loadHardwareSample() hwSample {
	hwCache.Lock()
	if time.Since(hwCache.at) < hwSampleTTL {
		v := hwCache.value
		hwCache.Unlock()
		return v
	}
	if wait := hwCache.running; wait != nil {
		hwCache.Unlock()
		<-wait
		hwCache.Lock()
		v := hwCache.value
		hwCache.Unlock()
		return v
	}
	done := make(chan struct{})
	hwCache.running = done
	hwCache.Unlock()

	v := hwSample{cpuTemp: loadCPUTemperature(), gpus: loadGPUBriefs()}

	hwCache.Lock()
	hwCache.value, hwCache.at, hwCache.running = v, time.Now(), nil
	hwCache.Unlock()
	close(done)
	return v
}

// cpuSensorHints are hwmon / thermal_zone names that belong to the CPU
// package (Intel coretemp, AMD k10temp/zenpower, ARM SoC zones).
var cpuSensorHints = []string{"coretemp", "k10temp", "zenpower", "x86_pkg_temp", "cpu", "soc", "package", "tctl", "tdie"}

// loadCPUTemperature returns the hottest CPU sensor in °C, or 0 when the
// host exposes none (typical for cloud VMs).
func loadCPUTemperature() float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	temps, _ := sensors.TemperaturesWithContext(ctx)
	return pickCPUTemperature(temps)
}

func pickCPUTemperature(temps []sensors.TemperatureStat) float64 {
	var maxCPU float64
	for _, t := range temps {
		// Discard unset / bogus readings.
		if t.Temperature <= 0 || t.Temperature > 150 {
			continue
		}
		key := strings.ToLower(t.SensorKey)
		for _, hint := range cpuSensorHints {
			if strings.Contains(key, hint) {
				if t.Temperature > maxCPU {
					maxCPU = t.Temperature
				}
				break
			}
		}
	}
	return maxCPU
}

func loadGPUBriefs() []dto.MonitorGPUBrief {
	ok, client := accelerator.New()
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), hwSampleTimeout)
	defer cancel()
	snapshot, err := client.Collect(ctx)
	if snapshot == nil || (err != nil && len(snapshot.Devices) == 0) {
		return nil
	}
	return gpuBriefsFromDevices(snapshot.Devices)
}

func gpuBriefsFromDevices(devices []accelerator.Device) []dto.MonitorGPUBrief {
	var out []dto.MonitorGPUBrief
	for _, d := range devices {
		// Sub-devices (NPU chips, XPU tiles) are folded into their parent.
		if d.ParentID != "" {
			continue
		}
		out = append(out, dto.MonitorGPUBrief{
			Index:       d.Index,
			Kind:        string(d.Kind),
			Vendor:      d.Vendor,
			Name:        d.Name,
			Utilization: d.Metrics.Utilization.Value,
			Temperature: d.Metrics.Temperature.Value,
			MemoryUsed:  d.Metrics.MemoryUsed.Value,
			MemoryTotal: d.Metrics.MemoryTotal.Value,
		})
	}
	return out
}

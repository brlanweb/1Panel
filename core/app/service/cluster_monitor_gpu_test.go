package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
)

func fptr(v float64) *float64 { return &v }

func TestClusterMonitorTemperatureRisks(t *testing.T) {
	s := &ClusterMonitorService{}
	res := s.withRisks(dto.ClusterMonitorStaticRes{Nodes: []dto.ClusterMonitorStatic{{
		NodeID: 3, Name: "gpu-a", Online: true, CpuTemperature: 83,
		GPUs: []dto.ClusterMonitorGPU{
			{Index: 0, Name: "A100", Utilization: fptr(100), Temperature: fptr(92)},
			{Index: 1, Name: "A100", Utilization: fptr(100), Temperature: fptr(60)},
			{Index: 2, Name: "A100"}, // tool reported nothing
		},
	}}})
	kinds := map[string]string{}
	for _, r := range res.Risks {
		kinds[r.Kind+r.Target] = r.Level
	}
	if kinds["cpuTemp"] != "warning" {
		t.Errorf("cpuTemp risk = %q, want warning at 83°C (risks %+v)", kinds["cpuTemp"], res.Risks)
	}
	if kinds["gpuTemp#0 A100"] != "critical" {
		t.Errorf("gpu #0 temperature risk = %q, want critical at 92°C", kinds["gpuTemp#0 A100"])
	}
	if len(res.Risks) != 2 {
		t.Errorf("risks = %+v, want only the two temperature risks (100%% GPU load is normal)", res.Risks)
	}
	if res.WarnTemp != monitorWarnTemp || res.CriticalTemp != monitorCritTemp {
		t.Errorf("thresholds = %v/%v", res.WarnTemp, res.CriticalTemp)
	}
}

func TestGpusFromAILoad(t *testing.T) {
	raw := `{"gpu":[{"index":0,"productName":"NVIDIA A100-SXM4-80GB","gpuUtil":"87 %","temperature":"64 C","memUsed":"40960 MiB","memTotal":"81920 MiB"},
	                  {"index":1,"productName":"NVIDIA GB10","gpuUtil":"N/A","temperature":"51 C","memUsed":"N/A","memTotal":"N/A"}],
	         "npu":[{"index":0,"productName":"910B","aiCore":"30 %","temperature":"55 C","hbmUsed":"2048 MiB","hbmTotal":"65536 MiB"}],
	         "xpu":[{"basic":{"deviceID":0,"deviceName":"Intel Arc","memory":"16 GiB"},"stats":{"gpuUtil":"12 %","temperature":"48","memoryUsed":"1024 MiB"}}]}`
	var data aiGPULoad
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	gpus := gpusFromAILoad(data)
	if len(gpus) != 4 {
		t.Fatalf("got %d devices, want 4: %+v", len(gpus), gpus)
	}
	a100 := gpus[0]
	if a100.Kind != "gpu" || *a100.Utilization != 87 || *a100.Temperature != 64 || *a100.MemoryUsed != 40960 || *a100.MemoryTotal != 81920 {
		t.Errorf("A100 = %+v", a100)
	}
	if gpus[1].Utilization != nil || gpus[1].MemoryTotal != nil || *gpus[1].Temperature != 51 {
		t.Errorf("GB10 N/A values must stay nil: %+v", gpus[1])
	}
	if gpus[2].Kind != "npu" || *gpus[2].Utilization != 30 || *gpus[2].MemoryTotal != 65536 {
		t.Errorf("NPU = %+v", gpus[2])
	}
	if gpus[3].Kind != "xpu" || *gpus[3].MemoryTotal != 16384 || *gpus[3].Temperature != 48 {
		t.Errorf("XPU = %+v", gpus[3])
	}
	if util, temp := gpuMaxima(gpus); util != 87 || temp != 64 {
		t.Errorf("maxima = %v / %v", util, temp)
	}
}

func TestLegacyMarkExpires(t *testing.T) {
	s := &ClusterMonitorService{legacyNodes: map[uint]time.Time{}}
	s.setLegacy(5)
	if !s.isLegacy(5) {
		t.Fatal("freshly marked node must be legacy")
	}
	s.legacyNodes[5] = time.Now().Add(-legacyRecheck - time.Second)
	if s.isLegacy(5) {
		t.Fatal("legacy mark must expire so an upgraded node gets GPU data again")
	}
}

func TestIsMissingRouteErr(t *testing.T) {
	if !isMissingRouteErr(errors.New("remote panel returned HTTP 404")) {
		t.Error("HTTP 404 is a missing route")
	}
	if isMissingRouteErr(errors.New("remote panel returned an unreadable response")) {
		t.Error("an HTML 502 while the node restarts must not mark the node legacy")
	}
}

func TestGpuMaxima(t *testing.T) {
	util, temp := gpuMaxima([]dto.ClusterMonitorGPU{
		{Utilization: fptr(30), Temperature: fptr(70)},
		{Utilization: fptr(95)},
		{Temperature: fptr(81)},
	})
	if util != 95 || temp != 81 {
		t.Fatalf("gpuMaxima = %v / %v, want 95 / 81", util, temp)
	}
}

package service

import (
	"testing"

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

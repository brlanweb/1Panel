package service

import (
	"testing"

	"github.com/1Panel-dev/1Panel/agent/utils/ai_tools/accelerator"
	"github.com/shirou/gopsutil/v4/sensors"
)

func TestPickCPUTemperature(t *testing.T) {
	got := pickCPUTemperature([]sensors.TemperatureStat{
		{SensorKey: "coretemp_package_id_0", Temperature: 61},
		{SensorKey: "coretemp_core_3", Temperature: 67},
		{SensorKey: "nvme_composite", Temperature: 75}, // not CPU
		{SensorKey: "k10temp_tctl", Temperature: 0},    // unset
		{SensorKey: "acpitz", Temperature: 200},        // bogus
	})
	if got != 67 {
		t.Fatalf("pickCPUTemperature = %v, want hottest CPU sensor 67", got)
	}
	if v := pickCPUTemperature(nil); v != 0 {
		t.Fatalf("no sensors = %v, want 0", v)
	}
}

func TestGPUBriefsSkipSubDevices(t *testing.T) {
	util, temp := 88.0, 71.0
	briefs := gpuBriefsFromDevices([]accelerator.Device{
		{Index: 0, Kind: accelerator.KindGPU, Vendor: "nvidia", Name: "RTX 4090",
			Metrics: accelerator.Metrics{Utilization: accelerator.Metric{Value: &util}, Temperature: accelerator.Metric{Value: &temp}}},
		{Index: 0, Kind: accelerator.KindNPU, ParentID: "npu-0", Name: "chip"},
	})
	if len(briefs) != 1 || briefs[0].Name != "RTX 4090" || *briefs[0].Utilization != 88 || *briefs[0].Temperature != 71 {
		t.Fatalf("briefs = %+v", briefs)
	}
}

package npu

import (
	"os"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/utils/re"
)

func loadAscendFixture(t *testing.T, name string) *Info {
	t.Helper()
	re.Init()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return parseAscendSMI(string(data))
}

// 310P cards list two chips under one card header; both must be kept.
func TestParseAscendMultiChipCard(t *testing.T) {
	info := loadAscendFixture(t, "npu-smi-310p.txt")
	if len(info.Devices) != 2 {
		t.Fatalf("want 2 chips, got %d: %+v", len(info.Devices), info.Devices)
	}
	for i, d := range info.Devices {
		if d.NPUIndex != 8 || d.ChipIndex != uint(i) || d.ProductName != "310P3" || d.Temperature != "42 C" {
			t.Fatalf("chip %d: %+v", i, d)
		}
	}
	if info.Devices[1].AICore != "15 %" || info.Devices[1].MemTotal != "43693 MB" {
		t.Fatalf("chip 1 metrics: %+v", info.Devices[1])
	}
}

func TestParseAscend910BWithProcesses(t *testing.T) {
	info := loadAscendFixture(t, "npu-smi-910b.txt")
	if info.DriverVersion != "23.0.6" || len(info.Devices) != 2 {
		t.Fatalf("info: %+v", info)
	}
	d := info.Devices[1]
	if d.Health != "Warning" || d.AICore != "87 %" || d.HBMUsed != "40211 MB" || d.HBMTotal != "65536 MB" || len(d.Processes) != 1 {
		t.Fatalf("device 1: %+v", d)
	}
}

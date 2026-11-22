package service

import (
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
)

func TestClusterSamplerEvaluate(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	s := &clusterAlertSampler{cpu: map[uint][]usageSample{}, memory: map[uint][]usageSample{}, lastReport: map[uint]time.Time{}}
	static := dto.ClusterMonitorStaticRes{Nodes: []dto.ClusterMonitorStatic{
		{NodeID: 0, Name: "local", Online: true, MemoryUsedPercent: 50, Disks: []dto.ClusterMonitorDisk{
			{Path: "/", UsedPercent: 95}, {Path: "/snap/core", UsedPercent: 100},
		}},
		{NodeID: 2, Name: "n2", Online: false, Error: "dial timeout"},
	}}
	// Five minutes of CPU: 80, 90, 100 → average 90.
	for i, v := range []float64{80, 90, 100} {
		at := now.Add(time.Duration(i-2) * time.Minute)
		s.cpu[0] = append(s.cpu[0], usageSample{at: at, value: v})
	}
	// An old sample outside the 5 min window must be ignored.
	s.cpu[0] = append([]usageSample{{at: now.Add(-20 * time.Minute), value: 0}}, s.cpu[0]...)
	s.memory[0] = []usageSample{{at: now, value: 50}}

	rule := clusterAlertRule{ID: 1, Cycle: 5}
	rule.Params.CPU, rule.Params.Memory, rule.Params.Disk, rule.Params.Offline = 85, 80, 90, true
	rule.Params.DiskExclude = []string{"/snap/*"}

	got := map[string]clusterAlertTarget{}
	for _, item := range s.evaluate(rule, static, now) {
		got[item.Key] = item
	}
	expect := map[string]bool{
		"node:0:offline": false,
		"node:0:cpu":     true,
		"node:0:memory":  false,
		"node:0:disk:/":  true,
		"node:2:offline": true,
	}
	if len(got) != len(expect) {
		t.Fatalf("unexpected targets: %+v", got)
	}
	for key, failed := range expect {
		item, ok := got[key]
		if !ok || item.Failed != failed {
			t.Fatalf("%s: want failed=%v, got %+v", key, failed, item)
		}
	}

	// nodeIds filter.
	rule.Params.NodeIDs = []uint{2}
	for _, item := range s.evaluate(rule, static, now) {
		if item.Key != "node:2:offline" {
			t.Fatalf("node filter leaked %s", item.Key)
		}
	}
}

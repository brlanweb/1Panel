package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/global"
)

// Cluster resource alert sampler.
//
// Runs in the core of every panel (the rules live in its own agent's alert
// DB, so a panel without clusterResource rules does nothing but one cheap
// local call every 30 seconds). It reuses the monitor page cache, keeps a
// short CPU / memory history per node to compute window averages, and posts
// the evaluated targets to the agent, which owns the alert state machine
// and the notification channels.

const (
	clusterSamplerTick     = 30 * time.Second
	clusterReportInterval  = 55 * time.Second
	clusterHistoryKeep     = 31 * time.Minute
	clusterSamplerCallTime = 10 * time.Second
)

type clusterAlertRule struct {
	ID        uint   `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Cycle     uint   `json:"cycle"`
	Count     uint   `json:"count"`
	SendCount uint   `json:"sendCount"`
	Params    struct {
		NodeIDs     []uint   `json:"nodeIds"`
		CPU         float64  `json:"cpu"`
		Memory      float64  `json:"memory"`
		Disk        float64  `json:"disk"`
		Offline     bool     `json:"offline"`
		DiskExclude []string `json:"diskExclude"`
	} `json:"params"`
}

type clusterAlertTarget struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Failed bool   `json:"failed"`
	Value  string `json:"value"`
	Error  string `json:"error"`
}

type clusterAlertCheck struct {
	AlertID uint                 `json:"alertId"`
	Targets []clusterAlertTarget `json:"targets"`
}

type usageSample struct {
	at    time.Time
	value float64
}

type clusterAlertSampler struct {
	mu         sync.Mutex
	cpu        map[uint][]usageSample
	memory     map[uint][]usageSample
	lastReport map[uint]time.Time
}

var clusterSamplerOnce sync.Once

// StartClusterAlertSampler starts the background sampler once.
func StartClusterAlertSampler() {
	clusterSamplerOnce.Do(func() {
		sampler := &clusterAlertSampler{
			cpu:        map[uint][]usageSample{},
			memory:     map[uint][]usageSample{},
			lastReport: map[uint]time.Time{},
		}
		go sampler.loop()
	})
}

func (s *clusterAlertSampler) loop() {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("cluster alert sampler panic: %v", r)
		}
	}()
	// Give the agent time to come up.
	time.Sleep(20 * time.Second)
	ticker := time.NewTicker(clusterSamplerTick)
	defer ticker.Stop()
	for {
		s.run(time.Now())
		<-ticker.C
	}
}

func (s *clusterAlertSampler) run(now time.Time) {
	var rules []clusterAlertRule
	ctx, cancel := context.WithTimeout(context.Background(), clusterSamplerCallTime)
	err := (&ClusterSyncService{}).callLocalJSON(ctx, http.MethodGet, "/api/v2/alert/cluster/rules", nil, &rules, clusterSamplerCallTime)
	cancel()
	if err != nil {
		global.LOG.Debugf("cluster alert sampler: load rules failed: %v", err)
		return
	}
	if len(rules) == 0 {
		s.mu.Lock()
		s.cpu, s.memory = map[uint][]usageSample{}, map[uint][]usageSample{}
		s.mu.Unlock()
		return
	}
	monitor := sharedClusterMonitor
	static, err := monitor.Static(false)
	if err != nil {
		global.LOG.Errorf("cluster alert sampler: static sample failed: %v", err)
		return
	}
	realtime, _ := monitor.Realtime()
	s.record(now, static, realtime)

	var checks []clusterAlertCheck
	for _, rule := range rules {
		s.mu.Lock()
		last := s.lastReport[rule.ID]
		due := now.Sub(last) >= clusterReportInterval
		if due {
			s.lastReport[rule.ID] = now
		}
		s.mu.Unlock()
		if !due {
			continue
		}
		if targets := s.evaluate(rule, static, now); len(targets) > 0 {
			checks = append(checks, clusterAlertCheck{AlertID: rule.ID, Targets: targets})
		}
	}
	if len(checks) == 0 {
		return
	}
	ctx, cancel = context.WithTimeout(context.Background(), clusterSamplerCallTime)
	defer cancel()
	body := map[string]interface{}{"checks": checks}
	if err := (&ClusterSyncService{}).callLocalJSON(ctx, http.MethodPost, "/api/v2/alert/cluster/report", body, nil, clusterSamplerCallTime); err != nil {
		global.LOG.Errorf("cluster alert sampler: report failed: %v", err)
	}
}

func (s *clusterAlertSampler) record(now time.Time, static dto.ClusterMonitorStaticRes, realtime dto.ClusterMonitorRealtimeRes) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := now.Add(-clusterHistoryKeep)
	push := func(store map[uint][]usageSample, id uint, value float64) {
		list := append(store[id], usageSample{at: now, value: value})
		start := 0
		for start < len(list) && list[start].at.Before(cutoff) {
			start++
		}
		store[id] = list[start:]
	}
	seen := map[uint]bool{}
	for _, node := range realtime.Nodes {
		if node.Online {
			push(s.cpu, node.NodeID, node.CpuUsedPercent)
		}
	}
	for _, node := range static.Nodes {
		seen[node.NodeID] = true
		if node.Online {
			push(s.memory, node.NodeID, node.MemoryUsedPercent)
		} else {
			// Do not average across an outage.
			delete(s.cpu, node.NodeID)
			delete(s.memory, node.NodeID)
		}
	}
	for id := range s.memory {
		if !seen[id] {
			delete(s.memory, id)
			delete(s.cpu, id)
		}
	}
}

func windowAverage(list []usageSample, now time.Time, window time.Duration) (float64, bool) {
	sum, n := 0.0, 0
	for _, item := range list {
		if now.Sub(item.at) <= window {
			sum += item.value
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

func (s *clusterAlertSampler) evaluate(rule clusterAlertRule, static dto.ClusterMonitorStaticRes, now time.Time) []clusterAlertTarget {
	include := map[uint]bool{}
	for _, id := range rule.Params.NodeIDs {
		include[id] = true
	}
	window := time.Duration(rule.Cycle) * time.Minute
	if window <= 0 {
		window = time.Minute
	}
	if window > 30*time.Minute {
		window = 30 * time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var targets []clusterAlertTarget
	for _, node := range static.Nodes {
		if len(include) > 0 && !include[node.NodeID] {
			continue
		}
		name := node.Name
		if node.NodeID == 0 {
			name = "master"
		}
		prefix := fmt.Sprintf("node:%d:", node.NodeID)
		if rule.Params.Offline {
			target := clusterAlertTarget{Key: prefix + "offline", Label: name + " online", Value: "online"}
			if !node.Online {
				target.Failed, target.Value = true, "offline"
				target.Error = firstNonEmpty(node.Error, "unreachable")
			}
			targets = append(targets, target)
		}
		// An unreachable node only reports "offline"; its metrics are left
		// untouched until it comes back.
		if !node.Online {
			continue
		}
		if rule.Params.CPU > 0 {
			if avg, ok := windowAverage(s.cpu[node.NodeID], now, window); ok {
				targets = append(targets, thresholdTarget(prefix+"cpu", name+" CPU", avg, rule.Params.CPU, rule.Cycle))
			}
		}
		if rule.Params.Memory > 0 {
			if avg, ok := windowAverage(s.memory[node.NodeID], now, window); ok {
				targets = append(targets, thresholdTarget(prefix+"memory", name+" memory", avg, rule.Params.Memory, rule.Cycle))
			}
		}
		if rule.Params.Disk > 0 {
			for _, disk := range node.Disks {
				if diskExcluded(disk.Path, rule.Params.DiskExclude) {
					continue
				}
				target := thresholdTarget(prefix+"disk:"+disk.Path, name+" disk "+disk.Path, disk.UsedPercent, rule.Params.Disk, 0)
				targets = append(targets, target)
			}
		}
	}
	return targets
}

func thresholdTarget(key, label string, value, threshold float64, windowMin uint) clusterAlertTarget {
	target := clusterAlertTarget{Key: key, Label: label, Value: fmt.Sprintf("%.1f%%", value)}
	if value >= threshold {
		target.Failed = true
		if windowMin > 0 {
			target.Error = fmt.Sprintf("%.1f%% avg %dmin ≥ %.0f%%", value, windowMin, threshold)
		} else {
			target.Error = fmt.Sprintf("%.1f%% ≥ %.0f%%", value, threshold)
		}
	}
	return target
}

func diskExcluded(path string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(pattern, "*")) {
				return true
			}
			continue
		}
		if path == pattern {
			return true
		}
	}
	return false
}

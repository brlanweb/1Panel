package service

import (
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
)

func newTestLogStore(capacity int) *computeCallLogStore {
	return &computeCallLogStore{ring: make([]dto.ComputeCallLog, capacity), minutes: map[int64]map[computeAggKey]*computeAgg{}}
}

func TestComputeCallLogRingAndPrune(t *testing.T) {
	s := newTestLogStore(3)
	now := time.Now()
	old := now.Add(-48 * time.Hour).UnixMilli()
	s.add(dto.ComputeCallLog{Time: old, Model: "m", Consumer: "a", Success: true})
	for i := 0; i < 3; i++ {
		s.add(dto.ComputeCallLog{Time: now.UnixMilli(), Model: "m", Consumer: "a", Success: true})
	}
	if s.size != 3 {
		t.Fatalf("ring must stay bounded, size=%d", s.size)
	}
	// Aggregates keep the dropped detail entry until pruned.
	if len(s.minutes) != 2 {
		t.Fatalf("minutes=%d", len(s.minutes))
	}
	s.prune(now) // default retention 24 h
	if len(s.minutes) != 1 {
		t.Fatalf("old minute not pruned: %d", len(s.minutes))
	}

	s = newTestLogStore(4)
	s.add(dto.ComputeCallLog{Time: old, Model: "m"})
	s.add(dto.ComputeCallLog{Time: old + 1, Model: "m"})
	s.add(dto.ComputeCallLog{Time: now.UnixMilli(), Model: "m"})
	s.add(dto.ComputeCallLog{Time: now.UnixMilli(), Model: "m"})
	s.prune(now)
	n := 0
	s.each(func(l *dto.ComputeCallLog) bool { n++; return true })
	if n != 2 || s.size != 2 {
		t.Fatalf("old ring entries not pruned: n=%d size=%d", n, s.size)
	}
}

func TestComputeAnalytics(t *testing.T) {
	saved := computeCallLogs
	defer func() { computeCallLogs = saved }()
	computeCallLogs = newTestLogStore(100)
	now := time.Now()
	add := func(ago time.Duration, m, c string, ok bool, ttft, lat, p, comp int64) {
		computeCallLogs.add(dto.ComputeCallLog{Time: now.Add(-ago).UnixMilli(), Model: m, Consumer: c, Success: ok,
			TTFTMs: ttft, LatencyMs: lat, PromptTokens: p, CompletionTokens: comp})
	}
	add(10*time.Minute, "ds", "new-api", true, 200, 1000, 100, 50)
	add(9*time.Minute, "ds", "new-api", true, 400, 3000, 100, 150)
	add(5*time.Minute, "qwen", "agent", false, 0, 50, 0, 0)
	add(1*time.Minute, "ds", "agent", true, 300, 2000, 10, 10)

	res := (&ComputeService{}).Analytics(dto.ComputeAnalyticsReq{Start: now.Add(-time.Hour).UnixMilli()})
	sum := res.Summary
	if sum.Requests != 4 || sum.Failures != 1 || sum.TotalTokens != 420 {
		t.Fatalf("summary: %+v", sum)
	}
	if sum.SuccessRate != 75 {
		t.Fatalf("success rate %v", sum.SuccessRate)
	}
	if sum.AvgTTFTMs != 300 { // failures without a first byte are not averaged
		t.Fatalf("ttft %v", sum.AvgTTFTMs)
	}
	if sum.AvgLatencyMs != (1000+3000+50+2000)/4.0 {
		t.Fatalf("latency %v", sum.AvgLatencyMs)
	}
	// RPM counts from the first request (~10 min), not the hour asked for.
	if sum.AvgRPM < 0.35 || sum.AvgRPM > 0.45 {
		t.Fatalf("rpm %v", sum.AvgRPM)
	}
	if len(res.ByModel) != 2 || res.ByModel[0].Name != "ds" || res.ByModel[0].Tokens != 420 {
		t.Fatalf("by model: %+v", res.ByModel)
	}
	var reqs int64
	for _, b := range res.Buckets {
		reqs += b.Requests
	}
	if reqs != 4 || res.BucketSec != 60 {
		t.Fatalf("buckets: %d reqs, %ds", reqs, res.BucketSec)
	}
	f := (&ComputeService{}).Analytics(dto.ComputeAnalyticsReq{Start: now.Add(-time.Hour).UnixMilli(), Consumer: "agent"})
	if f.Summary.Requests != 2 || len(f.Consumers) != 2 {
		t.Fatalf("filtered: %+v consumers=%v", f.Summary, f.Consumers)
	}
	page := (&ComputeService{}).PageCallLogs(dto.ComputeCallLogSearch{Page: 1, PageSize: 2, Status: "success"})
	if page.Total != 3 || len(page.Items) != 2 || page.Items[0].Model != "ds" || page.Items[0].Consumer != "agent" {
		t.Fatalf("page: %+v", page)
	}
}

func TestComputeUsageFilter(t *testing.T) {
	f := &computeUsageFilter{}
	in := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}\n\n" +
		"data: [DONE]\n\n"
	// Feed in odd pieces: events must be reassembled.
	var out []byte
	for i := 0; i < len(in); i += 7 {
		out = append(out, f.feed([]byte(in[i:min(i+7, len(in))]))...)
	}
	out = append(out, f.flush()...)
	got := string(out)
	if strings.Contains(got, "usage") || !strings.Contains(got, "\"hi\"") || !strings.HasSuffix(got, "data: [DONE]\n\n") {
		t.Fatalf("filtered stream: %q", got)
	}
	// A chunk carrying both choices and usage is kept.
	keep := "data: {\"choices\":[{\"delta\":{}}],\"usage\":{\"prompt_tokens\":1}}\n\n"
	if string(f.feed([]byte(keep))) != keep {
		t.Fatal("chunk with choices must be kept")
	}
	body, ok := computeStreamUsage([]byte(`{"model":"m","stream":true}`))
	if !ok || !strings.Contains(string(body), `"include_usage":true`) {
		t.Fatalf("inject: %s", body)
	}
	if _, ok := computeStreamUsage([]byte(`{"model":"m","stream":true,"stream_options":{"include_usage":false}}`)); ok {
		t.Fatal("explicit stream_options must be kept")
	}
}

func TestComputeHostFitsReplicas(t *testing.T) {
	single := model.ComputeProfile{NodeCount: 1, MaxPerHost: 3, MinMemGB: 20}
	use := &computeHostUse{insts: []string{"a", "b"}, ports: map[int]bool{8000: true, 8001: true}}
	if !computeHostFits(single, nil) || !computeHostFits(single, use) {
		t.Fatal("2 of 3 replicas: one more fits")
	}
	use.insts = append(use.insts, "c")
	if computeHostFits(single, use) {
		t.Fatal("3 of 3 replicas: full")
	}
	if computeHostFits(model.ComputeProfile{NodeCount: 1, MaxPerHost: 1}, &computeHostUse{insts: []string{"a"}}) {
		t.Fatal("max 1 per host")
	}
	if computeHostFits(single, &computeHostUse{insts: []string{"g"}, multi: true}) {
		t.Fatal("a host in a multi-host group takes no extra replica")
	}
	if computeHostFits(model.ComputeProfile{NodeCount: 2, MaxPerHost: 3}, &computeHostUse{insts: []string{"a"}}) {
		t.Fatal("multi-host plans need empty hosts")
	}
	// Ports skip the ones in use.
	p := computePickPort(model.ComputeProfile{Port: 8000}, []uint{999}, map[uint]*computeHostUse{999: use})
	if p != 8002 {
		t.Fatalf("port %d", p)
	}
}

func TestComputeTuneProfileShare(t *testing.T) {
	st := computeState.host(4242)
	computeState.mu.Lock()
	st.rt = dto.ComputeHostRealtime{Online: true, MemTotal: 120 << 30, MemAvailable: 100 << 30,
		GPUs: []dto.ComputeGPU{{Kind: "gpu", Name: "NVIDIA GB10", Unified: true}}}
	computeState.mu.Unlock()
	defer func() {
		computeState.mu.Lock()
		delete(computeState.hosts, 4242)
		computeState.mu.Unlock()
	}()
	rt := model.ComputeRuntime{Device: computeKindGPU, Backend: model.ComputeBackendScript}
	prof := computeTuneProfile(model.ComputeProfile{NodeCount: 1, MaxPerHost: 3, MinMemGB: 30, ExtraArgs: "--max-model-len 8192"}, rt, []uint{4242})
	if !strings.Contains(prof.ExtraArgs, "--gpu-memory-utilization 0.25") || !strings.HasPrefix(prof.ExtraArgs, "--max-model-len 8192") {
		t.Fatalf("args: %q", prof.ExtraArgs)
	}
	kept := computeTuneProfile(model.ComputeProfile{NodeCount: 1, MaxPerHost: 3, MinMemGB: 30, ExtraArgs: "--gpu-memory-utilization 0.3"}, rt, []uint{4242})
	if kept.ExtraArgs != "--gpu-memory-utilization 0.3" {
		t.Fatalf("explicit utilization must win: %q", kept.ExtraArgs)
	}
	one := computeTuneProfile(model.ComputeProfile{NodeCount: 1, MaxPerHost: 1, MinMemGB: 30}, rt, []uint{4242})
	if one.ExtraArgs != "" {
		t.Fatal("one replica per host: untouched")
	}
}

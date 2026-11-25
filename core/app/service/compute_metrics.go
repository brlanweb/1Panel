package service

import (
	"bufio"
	"strings"
)

// Prometheus text parser for inference engine metrics (vLLM, SGLang).
// Only the handful of series the scheduler needs are kept; histogram
// buckets are skipped. Values of the same metric are summed across label
// sets (several engines / data-parallel ranks), except gauges that are
// ratios, which are averaged.

type computeEngineSample struct {
	Running          float64 `json:"running"`
	Waiting          float64 `json:"waiting"`
	KVUsage          float64 `json:"kvUsage"` // 0..1
	PromptTokens     float64 `json:"promptTokens"`
	GenerationTokens float64 `json:"generationTokens"`
	TTFTSum          float64 `json:"-"`
	TTFTCount        float64 `json:"-"`
	QueueSum         float64 `json:"-"`
	QueueCount       float64 `json:"-"`
	E2ESum           float64 `json:"-"`
	E2ECount         float64 `json:"-"`
	RequestSuccess   float64 `json:"requestSuccess"`
	GPUMemUtil       float64 `json:"gpuMemUtil"`
	ModelName        string  `json:"modelName"`
	Engine           string  `json:"engine"`
	Found            bool    `json:"found"`
}

type computeMetricKind int

const (
	cmSum computeMetricKind = iota
	cmAvg
)

type computeMetricMap struct {
	field func(*computeEngineSample) *float64
	kind  computeMetricKind
}

var computeMetricFields = map[string]computeMetricMap{
	// vLLM (v0 and v1 names)
	"vllm:num_requests_running":              {func(s *computeEngineSample) *float64 { return &s.Running }, cmSum},
	"vllm:num_requests_waiting":              {func(s *computeEngineSample) *float64 { return &s.Waiting }, cmSum},
	"vllm:kv_cache_usage_perc":               {func(s *computeEngineSample) *float64 { return &s.KVUsage }, cmAvg},
	"vllm:gpu_cache_usage_perc":              {func(s *computeEngineSample) *float64 { return &s.KVUsage }, cmAvg},
	"vllm:prompt_tokens_total":               {func(s *computeEngineSample) *float64 { return &s.PromptTokens }, cmSum},
	"vllm:generation_tokens_total":           {func(s *computeEngineSample) *float64 { return &s.GenerationTokens }, cmSum},
	"vllm:time_to_first_token_seconds_sum":   {func(s *computeEngineSample) *float64 { return &s.TTFTSum }, cmSum},
	"vllm:time_to_first_token_seconds_count": {func(s *computeEngineSample) *float64 { return &s.TTFTCount }, cmSum},
	"vllm:request_queue_time_seconds_sum":    {func(s *computeEngineSample) *float64 { return &s.QueueSum }, cmSum},
	"vllm:request_queue_time_seconds_count":  {func(s *computeEngineSample) *float64 { return &s.QueueCount }, cmSum},
	"vllm:e2e_request_latency_seconds_sum":   {func(s *computeEngineSample) *float64 { return &s.E2ESum }, cmSum},
	"vllm:e2e_request_latency_seconds_count": {func(s *computeEngineSample) *float64 { return &s.E2ECount }, cmSum},
	"vllm:request_success_total":             {func(s *computeEngineSample) *float64 { return &s.RequestSuccess }, cmSum},
	// SGLang
	"sglang:num_running_reqs":                  {func(s *computeEngineSample) *float64 { return &s.Running }, cmSum},
	"sglang:num_queue_reqs":                    {func(s *computeEngineSample) *float64 { return &s.Waiting }, cmSum},
	"sglang:token_usage":                       {func(s *computeEngineSample) *float64 { return &s.KVUsage }, cmAvg},
	"sglang:prompt_tokens_total":               {func(s *computeEngineSample) *float64 { return &s.PromptTokens }, cmSum},
	"sglang:generation_tokens_total":           {func(s *computeEngineSample) *float64 { return &s.GenerationTokens }, cmSum},
	"sglang:time_to_first_token_seconds_sum":   {func(s *computeEngineSample) *float64 { return &s.TTFTSum }, cmSum},
	"sglang:time_to_first_token_seconds_count": {func(s *computeEngineSample) *float64 { return &s.TTFTCount }, cmSum},
	"sglang:e2e_request_latency_seconds_sum":   {func(s *computeEngineSample) *float64 { return &s.E2ESum }, cmSum},
	"sglang:e2e_request_latency_seconds_count": {func(s *computeEngineSample) *float64 { return &s.E2ECount }, cmSum},
}

// parseComputeMetrics parses an exposition body. If modelFilter is not
// empty only series whose model_name label equals it are kept (one
// endpoint normally serves one model, so the filter is a safety net).
func parseComputeMetrics(body, modelFilter string) computeEngineSample {
	var s computeEngineSample
	avgCount := map[string]int{}
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		name, labels, value, ok := splitPromLine(line)
		if !ok {
			continue
		}
		if name == "vllm:cache_config_info" {
			if v := promLabel(labels, "gpu_memory_utilization"); v != "" {
				s.GPUMemUtil = cAtoF(v)
			}
			s.Engine = "vllm"
			continue
		}
		m, ok := computeMetricFields[name]
		if !ok {
			continue
		}
		model := promLabel(labels, "model_name")
		if modelFilter != "" && model != "" && model != modelFilter {
			continue
		}
		if s.ModelName == "" && model != "" {
			s.ModelName = model
		}
		if s.Engine == "" {
			s.Engine = strings.SplitN(name, ":", 2)[0]
		}
		s.Found = true
		v := cAtoF(value)
		ptr := m.field(&s)
		if m.kind == cmAvg {
			n := avgCount[name]
			*ptr = (*ptr*float64(n) + v) / float64(n+1)
			avgCount[name] = n + 1
		} else {
			*ptr += v
		}
	}
	if s.KVUsage > 1 { // some versions report percent
		s.KVUsage /= 100
	}
	return s
}

// splitPromLine splits `name{labels} value [timestamp]`.
func splitPromLine(line string) (name, labels, value string, ok bool) {
	rest := line
	if i := strings.IndexByte(line, '{'); i >= 0 {
		j := strings.LastIndexByte(line, '}')
		if j < i {
			return "", "", "", false
		}
		name, labels, rest = line[:i], line[i+1:j], strings.TrimSpace(line[j+1:])
	} else {
		sp := strings.IndexAny(line, " \t")
		if sp < 0 {
			return "", "", "", false
		}
		name, rest = line[:sp], strings.TrimSpace(line[sp:])
	}
	f := strings.Fields(rest)
	if len(f) == 0 {
		return "", "", "", false
	}
	return name, labels, f[0], true
}

func promLabel(labels, key string) string {
	for len(labels) > 0 {
		eq := strings.IndexByte(labels, '=')
		if eq < 0 || eq+1 >= len(labels) || labels[eq+1] != '"' {
			return ""
		}
		k := strings.TrimSpace(strings.TrimLeft(labels[:eq], ","))
		// find closing quote, honouring escapes
		i := eq + 2
		var b strings.Builder
		for i < len(labels) {
			c := labels[i]
			if c == '\\' && i+1 < len(labels) {
				b.WriteByte(labels[i+1])
				i += 2
				continue
			}
			if c == '"' {
				break
			}
			b.WriteByte(c)
			i++
		}
		if k == key {
			return b.String()
		}
		if i+1 >= len(labels) {
			return ""
		}
		labels = labels[i+1:]
	}
	return ""
}

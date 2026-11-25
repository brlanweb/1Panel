package service

import (
	"strings"
	"testing"
	"time"
)

// Fixtures follow what DGX-A-121 (GB10, Ubuntu 24.04, driver 580.159.03)
// actually returns.

const dgxFastOut = `@@STAT
cpu  1000 0 500 8000 500 0 0 0 0 0
4.70 4.52 4.40 3/1200 12345
20
@@MEM
MemTotal:       124608256 kB
MemAvailable:   10754048 kB
SwapTotal:       0 kB
SwapFree:        0 kB
@@GPU
0, NVIDIA GB10, 93, 71, 38.52, [N/A], [N/A]
@@NET
enP7s7 up 10000 1000000 2000000
enp1s0f1np1 up 200000 5000 6000
enp1s0f0np0 down -1 0 0
@@RDMA
rocep1s0f0 1 DOWN 0 enp1s0f0np0 0 0 0 0 0 0 0
rocep1s0f1 1 ACTIVE 200 enp1s0f1np1 1000 2000 0 0 0 0 0
@@DISK
/dev/nvme0n1p2 3842744340 1460302556 2187142616 41% /
`

func TestParseComputeFastDGX(t *testing.T) {
	r := parseComputeFast(dgxFastOut)
	if r.Cores != 20 || r.Load1 != 4.70 {
		t.Fatalf("cores/load: %+v", r)
	}
	if r.MemTotal != 124608256*1024 || r.MemAvailable != 10754048*1024 {
		t.Fatalf("mem: %d %d", r.MemTotal, r.MemAvailable)
	}
	if len(r.GPUs) != 1 || r.GPUs[0].Name != "NVIDIA GB10" || r.GPUs[0].Util != 93 || r.GPUs[0].MemTotal != 0 {
		t.Fatalf("gpu: %+v", r.GPUs)
	}
	if !computeIsUnified(r.GPUs[0]) {
		t.Fatal("GB10 must be detected as unified memory")
	}
	if len(r.Rdma) != 2 || r.Rdma[1].State != "ACTIVE" || r.Rdma[1].RateGbps != 200 || r.Rdma[1].NetDev != "enp1s0f1np1" {
		t.Fatalf("rdma: %+v", r.Rdma)
	}
	if r.DiskTotal == 0 || r.DiskUsed == 0 {
		t.Fatal("disk")
	}

	// Second sample: counters advance, an RDMA error appears.
	cur := strings.Replace(dgxFastOut, "cpu  1000 0 500 8000 500", "cpu  1500 0 700 8200 500", 1)
	cur = strings.Replace(cur, "1000 2000 0 0 0 0 0", "251000 502000 3 0 0 0 0", 1)
	r2 := parseComputeFast(cur)
	rt := computeBuildRealtime(r2, &r, 10*time.Second, map[string]string{"enP7s7": "10.15.9.121"})
	if rt.CpuPercent != 77.78 { // (700 busy)/(900 total)
		t.Fatalf("cpu percent %.2f", rt.CpuPercent)
	}
	g := rt.GPUs[0]
	if !g.Unified || g.MemTotal != r2.MemTotal || g.MemUsed != r2.MemTotal-r2.MemAvailable {
		t.Fatalf("unified gpu mem: %+v", g)
	}
	var link = rt.Rdma[1]
	if link.RxRate != 100000 || link.TxRate != 200000 || link.ErrDelta != 3 {
		t.Fatalf("rdma rate/err: %+v", link)
	}
	if rt.Nics[0].IP != "10.15.9.121" {
		t.Fatalf("nic ip: %+v", rt.Nics[0])
	}
	found := false
	for _, w := range rt.Warnings {
		if strings.HasPrefix(w, "rdmaErr:rocep1s0f1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing rdma warning: %v", rt.Warnings)
	}
}

const dgxSlowOut = `@@META
spark-121
aarch64
Ubuntu 24.04.4 LTS
NVRM version: NVIDIA UNIX Open Kernel Module for aarch64  580.159.03  Release Build
20
@@ADDR
enP7s7 10.15.9.121/24
enp1s0f1np1 192.168.103.10/24
enP2p1s0f1np1 192.168.104.10/24
docker0 172.17.0.1/16
@@ARP
192.168.103.11 4c:bb:47:7e:33:8a enp1s0f1np1
10.15.9.1 44:67:47:9d:bf:ac enP7s7
192.168.104.11 4c:bb:47:7e:33:8e enP2p1s0f1np1
10.15.9.123 4c:bb:47:7e:aa:e4 enP7s7
10.15.9.122 4c:bb:47:7e:33:89 enP7s7
@@CTR
vllm-ds|nvcr.io/nvidia/vllm:25.09-py3|Up 3 days
@@PROC
2101 /usr/bin/python3 /usr/local/bin/vllm serve /models/DeepSeek-V4-Flash-0731 --served-model-name deepseek-v4-flash-0731 --port 8000 --max-model-len 200000
2102 /usr/bin/python3 /usr/local/bin/vllm serve /models/DeepSeek-V4-Flash-0731 --served-model-name deepseek-v4-flash-0731 --port 8000 --max-model-len 200000
2203 python3 -m vllm.entrypoints.openai.api_server --model=/models/DeepSeek-V4-Flash-0731 --port=8101
`

func TestParseComputeSlowDGX(t *testing.T) {
	r := parseComputeSlow(dgxSlowOut)
	if r.Hostname != "spark-121" || r.Arch != "aarch64" || r.Driver != "580.159.03" || r.Cores != 20 {
		t.Fatalf("meta: %+v", r)
	}
	if r.Addrs["enp1s0f1np1"] != "192.168.103.10" || len(r.Neighbours) != 5 {
		t.Fatalf("addr/arp: %+v", r)
	}
	if len(r.Procs) != 2 {
		t.Fatalf("procs (dedup by port): %+v", r.Procs)
	}
	if p := r.Procs[0]; p.Engine != "vllm" || p.Port != 8000 || p.ServedName != "deepseek-v4-flash-0731" {
		t.Fatalf("proc0: %+v", p)
	}
	if p := r.Procs[1]; p.Port != 8101 || p.ServedName != "/models/DeepSeek-V4-Flash-0731" {
		t.Fatalf("proc1: %+v", p)
	}
	if len(r.Containers) != 1 || r.Containers[0].Name != "vllm-ds" {
		t.Fatalf("containers: %+v", r.Containers)
	}
}

const dgxMetrics = `# HELP vllm:num_requests_running Number of requests in model execution batches.
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running{engine="0",model_name="deepseek-v4-flash-0731"} 3.0
# TYPE vllm:num_requests_waiting gauge
vllm:num_requests_waiting{engine="0",model_name="deepseek-v4-flash-0731"} 1.0
# TYPE vllm:kv_cache_usage_perc gauge
vllm:kv_cache_usage_perc{engine="0",model_name="deepseek-v4-flash-0731"} 0.8212
vllm:prompt_tokens_total{engine="0",model_name="deepseek-v4-flash-0731"} 1.2e+06
vllm:generation_tokens_total{engine="0",model_name="deepseek-v4-flash-0731"} 340000.0
vllm:time_to_first_token_seconds_bucket{engine="0",le="0.1",model_name="deepseek-v4-flash-0731"} 10.0
vllm:time_to_first_token_seconds_sum{engine="0",model_name="deepseek-v4-flash-0731"} 120.5
vllm:time_to_first_token_seconds_count{engine="0",model_name="deepseek-v4-flash-0731"} 400.0
vllm:request_queue_time_seconds_sum{engine="0",model_name="deepseek-v4-flash-0731"} 8.0
vllm:request_queue_time_seconds_count{engine="0",model_name="deepseek-v4-flash-0731"} 400.0
vllm:cache_config_info{block_size="16",cache_dtype="fp8",gpu_memory_utilization="0.8",num_gpu_blocks="12000"} 1.0
`

func TestParseComputeMetricsVLLM(t *testing.T) {
	s := parseComputeMetrics(dgxMetrics, "deepseek-v4-flash-0731")
	if !s.Found || s.Running != 3 || s.Waiting != 1 || s.KVUsage != 0.8212 {
		t.Fatalf("gauges: %+v", s)
	}
	if s.PromptTokens != 1.2e6 || s.GenerationTokens != 340000 || s.TTFTCount != 400 || s.TTFTSum != 120.5 {
		t.Fatalf("counters: %+v", s)
	}
	if s.GPUMemUtil != 0.8 || s.ModelName != "deepseek-v4-flash-0731" || s.Engine != "vllm" {
		t.Fatalf("info: %+v", s)
	}
	if other := parseComputeMetrics(dgxMetrics, "glm-5"); other.Found {
		t.Fatal("model filter must drop other models")
	}
}

func TestComputeProcEdgeCases(t *testing.T) {
	if _, ok := parseComputeProc("99 grep vllm"); ok {
		t.Fatal("grep is not an engine")
	}
	p, ok := parseComputeProc("77 python3 -m sglang.launch_server --model-path /m/glm --served-model-name glm-5")
	if !ok || p.Engine != "sglang" || p.Port != 30000 || p.ServedName != "glm-5" {
		t.Fatalf("sglang: %+v %v", p, ok)
	}
}

func TestComputeTemplatesRender(t *testing.T) {
	for _, rt := range computeBuiltinRuntimes() {
		for _, rank := range []int{0, 1} {
			v := computeTplVars{Name: "ds@a", ContainerName: "xpanel-ai-ds-a", ServedName: "deepseek-v4-flash-0731",
				ModelPath: "/models/DeepSeek V4", ModelDir: "/models", Port: 8000, Image: rt.Image, APIKey: "sk-node-x",
				ExtraArgs: "--max-model-len 200000", Env: map[string]string{"A": "b c"}, NodeCount: 2, Rank: rank, IsHead: rank == 0,
				HostAddr: "10.15.9.121", HeadAddr: "10.15.9.121", FabricIface: "enp1s0f1np1", FabricIP: "192.168.103.10",
				HeadFabricIP: "192.168.103.10", RdmaDev: "rocep1s0f1"}
			out, err := computeRender(rt.Name, rt.LaunchTemplate, v)
			if err != nil {
				t.Fatalf("%s: %v", rt.Name, err)
			}
			if !strings.Contains(out, "docker run") || !strings.Contains(out, "'/models/DeepSeek V4'") && rank == 0 && !rt.MultiNode {
				t.Fatalf("%s rank %d:\n%s", rt.Name, rank, out)
			}
			if rt.MultiNode && rank == 1 && !strings.Contains(out, "--address=192.168.103.10:6379") {
				t.Fatalf("worker must join head over fabric:\n%s", out)
			}
			if rt.MultiNode && rank == 0 && !strings.Contains(out, "NCCL_IB_HCA=rocep1s0f1") {
				t.Fatalf("head must pin the RDMA device:\n%s", out)
			}
		}
		if _, err := computeRender("stop", rt.StopTemplate, computeTplVars{ContainerName: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if computeShellQuote("a'b") != `'a'"'"'b'` {
		t.Fatal("quote")
	}
}

func TestComputeUsageSniffer(t *testing.T) {
	s := &computeUsageSniffer{}
	body := "data: {\"choices\":[]}\n\ndata: {\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":34,\"total_tokens\":46}}\n\ndata: [DONE]\n\n"
	// feed in awkward chunks
	for i := 0; i < len(body); i += 7 {
		end := i + 7
		if end > len(body) {
			end = len(body)
		}
		s.feed([]byte(body[i:end]))
	}
	if s.prompt != 12 || s.completion != 34 {
		t.Fatalf("usage %d %d", s.prompt, s.completion)
	}
}

func TestPromLabel(t *testing.T) {
	l := `a="1",model_name="x\"y",b="2"`
	if promLabel(l, "model_name") != `x"y` || promLabel(l, "b") != "2" || promLabel(l, "c") != "" {
		t.Fatal("promLabel")
	}
}

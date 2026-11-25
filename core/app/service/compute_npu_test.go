package service

import (
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
)

// npu-smi info layouts: 910B (one chip per card, HBM pool, process table)
// and 310P (two chips under one card header, DDR pool, power N/A).
const npu910B = `+------------------------------------------------------------------------------------------------+
| npu-smi 23.0.6                   Version: 23.0.6                                               |
+---------------------------+---------------+----------------------------------------------------+
| NPU   Name                | Health        | Power(W)    Temp(C)           Hugepages-Usage(page)|
| Chip                      | Bus-Id        | AICore(%)   Memory-Usage(MB)  HBM-Usage(MB)        |
+===========================+===============+====================================================+
| 0     910B3               | OK            | 95.7        36                0    / 0             |
| 0                         | 0000:C1:00.0  | 0           0    / 0          3399 / 65536         |
+===========================+===============+====================================================+
| 1     910B3               | Warning       | 190.1       88                0    / 0             |
| 0                         | 0000:C2:00.0  | 87          0    / 0          40211/ 65536         |
+===========================+===============+====================================================+
+---------------------------+---------------+----------------------------------------------------+
| NPU     Chip              | Process id    | Process name             | Process memory(MB)      |
+===========================+===============+====================================================+
| 1       0                 | 2345          | python3                  | 37000                   |
+===========================+===============+====================================================+`

const npu310P = `| npu-smi 23.0.0                                   Version: 23.0.0                                       |
| NPU     Name                  | Health          | Power(W)     Temp(C)           Hugepages-Usage(page) |
| Chip    Device                | Bus-Id          | AICore(%)    Memory-Usage(MB)                        |
+===============================+=================+======================================================+
| 8       310P3                 | OK              | 30.4         42                0    / 0              |
| 0       0                     | 0000:01:00.0    | 0            1782 / 44280                            |
| 1       1                     | 0000:01:00.0    | 15           1543 / 43693                            |
+===============================+=================+======================================================+`

func TestParseComputeNPU910B(t *testing.T) {
	out := strings.Replace(dgxFastOut, "@@GPU\n0, NVIDIA GB10, 93, 71, 38.52, [N/A], [N/A]\n", "@@GPU\n@@NPU\n"+npu910B+"\n", 1)
	r := parseComputeFast(out)
	if len(r.GPUs) != 2 {
		t.Fatalf("want 2 NPUs, got %+v", r.GPUs)
	}
	a, b := r.GPUs[0], r.GPUs[1]
	if a.Kind != "npu" || a.Name != "910B3" || a.Card != 0 || a.Power != 95.7 || a.Temp != 36 || a.MemTotal != 65536 || a.MemUsed != 3399 {
		t.Fatalf("npu0: %+v", a)
	}
	if b.Health != "Warning" || b.Util != 87 || b.MemUsed != 40211 || b.Card != 1 {
		t.Fatalf("npu1: %+v", b)
	}
	if computeIsUnified(a) {
		t.Fatal("NPU HBM is dedicated memory, not unified")
	}

	rt := computeBuildRealtime(r, nil, 0, nil)
	if rt.GPUs[0].Kind != "npu" || rt.GPUs[0].MemTotal != 65536<<20 || rt.GPUs[0].Unified {
		t.Fatalf("realtime: %+v", rt.GPUs[0])
	}
	joined := strings.Join(rt.Warnings, ",")
	if !strings.Contains(joined, "npuTemp:1:88") || !strings.Contains(joined, "npuHealth:1-0:Warning") {
		t.Fatalf("warnings: %v", rt.Warnings)
	}

	// Placement memory: free HBM of healthy chips only (chip 1 is Warning).
	free, pool := computeFreeMem(rt, "npu", false)
	if pool != "NPU HBM" || free != (65536-3399)<<20 {
		t.Fatalf("free %d %s", free, pool)
	}
	if computeAccelCount(rt, "npu") != 1 || computeAccelCount(rt, "gpu") != 0 {
		t.Fatal("accelerator count by kind")
	}
}

func TestParseComputeNPU310PMultiChip(t *testing.T) {
	g := parseComputeNPU(strings.Split(npu310P, "\n"))
	if len(g) != 2 {
		t.Fatalf("want 2 chips, got %+v", g)
	}
	for i, c := range g {
		if c.Card != 8 || c.Chip != i || c.Index != i || c.Name != "310P3" || c.Temp != 42 || c.Power != 15.2 {
			t.Fatalf("chip %d: %+v", i, c)
		}
	}
	if g[1].Util != 15 || g[1].MemUsed != 1543 || g[1].MemTotal != 43693 {
		t.Fatalf("chip1 metrics: %+v", g[1])
	}
}

func TestComputeFreeMemUnifiedGPU(t *testing.T) {
	r := parseComputeFast(dgxFastOut)
	rt := computeBuildRealtime(r, nil, time.Second, nil)
	free, pool := computeFreeMem(rt, "gpu", false)
	if pool != "RAM" || free != r.MemAvailable {
		t.Fatalf("GB10 must use host memory: %d %s", free, pool)
	}
	if n := computeAccelCount(rt, "npu"); n != 0 {
		t.Fatal("no NPU on DGX")
	}
	var empty dto.ComputeHostRealtime
	if computeAccelCount(empty, "gpu") != 0 {
		t.Fatal("empty")
	}
}

func TestComputeAscendDriverVersion(t *testing.T) {
	out := "@@META\nnpu-01\naarch64\nopenEuler 22.03\nVersion=24.1.rc2\n192\n"
	if d := parseComputeSlow(out).Driver; d != "24.1.rc2" {
		t.Fatalf("driver %q", d)
	}
}

func TestComputeAscendTemplateRenders(t *testing.T) {
	for _, rt := range computeBuiltinRuntimes() {
		if rt.Name != "vllm-ascend" {
			continue
		}
		out, err := computeRender(rt.Name, rt.LaunchTemplate, computeTplVars{ContainerName: "c", ServedName: "qwen3", ModelPath: "/m/q",
			ModelDir: "/m", Port: 8000, Image: rt.Image, Env: map[string]string{"ASCEND_VISIBLE_DEVICES": "0-7"}})
		if err != nil || !strings.Contains(out, "-e ASCEND_VISIBLE_DEVICES=0-7") || strings.Contains(out, "--gpus") {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	t.Fatal("vllm-ascend runtime missing")
}

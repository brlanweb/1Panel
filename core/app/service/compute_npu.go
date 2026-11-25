package service

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/1Panel-dev/1Panel/core/app/dto"
)

// Ascend NPU support for the agentless collector.
//
// `npu-smi info` prints one table: a card header row
//   | <npu> <name> | <health> | <power> <temp> <hugepages used / total> |
// followed by one row per chip
//   | <chip> [<device>] | <bus id> | <aicore%> <mem used / total> [<hbm used / total>] |
// 910B has one chip per card (HBM is the usable pool), 310P and A3 have two.
// The process section after the table is ignored here (the slow tier
// already lists engine processes). Core and agent are separate modules, so
// this is a compact copy of agent/utils/ai_tools/npu.

var computeNPUPairRe = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*/\s*([0-9]+(?:\.[0-9]+)?)`)

func computeNPUCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return nil
	}
	cells := make([]string, 0, len(parts)-2)
	for _, p := range parts[1 : len(parts)-1] {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

func computeNPUNum(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "NA") || strings.EqualFold(s, "N/A") {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func parseComputeNPU(lines []string) []computeGPURaw {
	type card struct {
		id           int
		name, health string
		power, temp  float64
	}
	var (
		res []computeGPURaw
		cur *card
	)
	for _, line := range lines {
		if strings.Contains(line, "Process id") {
			break
		}
		cells := computeNPUCells(line)
		if len(cells) != 3 {
			continue
		}
		first := strings.Fields(cells[0])
		if len(first) == 0 {
			continue
		}
		id, err := strconv.Atoi(first[0])
		if err != nil {
			continue // header rows ("NPU Name", "Chip Device")
		}
		// Card header: second field is the (non-numeric) product name.
		if len(first) >= 2 {
			if _, err := strconv.Atoi(first[1]); err != nil {
				m := strings.Fields(cells[2])
				c := &card{id: id, name: strings.Join(first[1:], " "), health: cells[1]}
				if len(m) > 0 {
					c.power, _ = computeNPUNum(m[0])
				}
				if len(m) > 1 {
					c.temp, _ = computeNPUNum(m[1])
				}
				cur = c
				continue
			}
		}
		if cur == nil {
			continue
		}
		m := strings.Fields(cells[2])
		if len(m) == 0 {
			continue
		}
		util, _ := computeNPUNum(m[0])
		item := computeGPURaw{Kind: "npu", Index: len(res), Card: cur.id, Chip: id, Name: cur.name,
			Health: cur.health, Util: util, Temp: cur.temp, Power: cur.power}
		if len(first) > 1 {
			if dev, err := strconv.Atoi(first[1]); err == nil {
				item.Index = dev
			}
		}
		// Prefer the last pool with capacity (HBM on 910B, DDR on 310P).
		pairs := computeNPUPairRe.FindAllStringSubmatch(cells[2], -1)
		for i := len(pairs) - 1; i >= 0; i-- {
			total := cAtoF(pairs[i][2])
			if total > 0 {
				item.MemUsed, item.MemTotal = cAtoF(pairs[i][1]), total
				break
			}
		}
		res = append(res, item)
	}
	// Power is reported per card: split it across the card's chips so
	// host totals are not double counted.
	chips := map[int]int{}
	for _, g := range res {
		chips[g.Card]++
	}
	for i := range res {
		if n := chips[res[i].Card]; n > 1 {
			res[i].Power = computeRound(res[i].Power / float64(n))
		}
	}
	return res
}

const (
	computeKindGPU = "gpu"
	computeKindNPU = "npu"
)

func computeDeviceKind(device string) string {
	if device == computeKindNPU {
		return computeKindNPU
	}
	return computeKindGPU
}

// computeAccelCount counts the healthy accelerators of one kind on a host.
func computeAccelCount(rt dto.ComputeHostRealtime, device string) int {
	kind := computeDeviceKind(device)
	n := 0
	for _, g := range rt.GPUs {
		k := g.Kind
		if k == "" {
			k = computeKindGPU
		}
		if k == kind && (kind != computeKindNPU || computeNPUHealthy(g.Health)) {
			n++
		}
	}
	return n
}

// computeFreeMem returns the memory pool that bounds model placement:
//   - NPU: free HBM / device memory summed over healthy chips (the model
//     weights and KV cache live there, host RAM is irrelevant);
//   - GPU with dedicated memory: free VRAM summed over cards;
//   - unified memory (GB10 …): host MemAvailable.
//
// ignoreUsage returns capacity instead of free space (a switch frees the
// old model first).
func computeFreeMem(rt dto.ComputeHostRealtime, device string, ignoreUsage bool) (uint64, string) {
	kind := computeDeviceKind(device)
	var total, used uint64
	dedicated := false
	for _, g := range rt.GPUs {
		k := g.Kind
		if k == "" {
			k = computeKindGPU
		}
		if k != kind || g.Unified || g.MemTotal == 0 {
			continue
		}
		if kind == computeKindNPU && !computeNPUHealthy(g.Health) {
			continue
		}
		dedicated = true
		total += g.MemTotal
		used += min(g.MemUsed, g.MemTotal)
	}
	if dedicated {
		pool := "VRAM"
		if kind == computeKindNPU {
			pool = "NPU HBM"
		}
		if ignoreUsage {
			return total, pool
		}
		return total - used, pool
	}
	if ignoreUsage {
		return rt.MemTotal, "RAM"
	}
	return rt.MemAvailable, "RAM"
}

// computeNPUHealthy: npu-smi reports OK / Warning / Alarm / Critical.
func computeNPUHealthy(h string) bool {
	h = strings.ToUpper(strings.TrimSpace(h))
	return h == "" || h == "OK"
}

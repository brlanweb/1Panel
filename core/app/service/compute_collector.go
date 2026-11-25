package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/global"
)

// Tiered collector.
//
//   fast  (15s, 5s while watched): CPU / memory / GPU / NIC / RDMA counters
//   slow  (5 min, or on demand):   identity, addresses, neighbours,
//                                  containers, engine processes -> topology
//   engine metrics (15s, 5s while watched): HTTP GET <endpoint>/metrics
//
// A host with no instance and nobody watching is sampled at the slow
// cadence only (every 60s fast), so an idle fleet costs almost nothing.
// Database writes happen only on status changes and once per slow round.

const (
	computeTick          = 5 * time.Second
	computeFastIdle      = 15 * time.Second
	computeFastWatch     = 5 * time.Second
	computeFastQuiet     = 60 * time.Second
	computeSlowInterval  = 5 * time.Minute
	computeFanout        = 8
	computeFastTimeout   = 20 * time.Second
	computeSlowTimeout   = 30 * time.Second
	computeMetricTimeout = 4 * time.Second
	computeOfflineAfter  = 2
)

var (
	computeStartOnce sync.Once
	computeKickCh    = make(chan uint, 64)
	computeHTTP      = &http.Client{Timeout: computeMetricTimeout, Transport: &http.Transport{
		MaxIdleConnsPerHost: 4, IdleConnTimeout: 90 * time.Second, DisableCompression: false,
	}}
)

// StartComputeScheduler starts the collector, the scheduler and (when
// enabled) the model router. Safe to call more than once.
func StartComputeScheduler() {
	computeStartOnce.Do(func() {
		computeSeedRuntimes()
		go computeSSH.reapLoop()
		go computeCollectLoop()
		go computeSchedulerLoop()
		go func() {
			time.Sleep(3 * time.Second)
			if err := computeRouterApply(); err != nil {
				global.LOG.Errorf("compute router start failed: %v", err)
			}
		}()
	})
}

func computeKick(hostID uint) {
	select {
	case computeKickCh <- hostID:
	default:
	}
}

func computeCollectLoop() {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("compute collector panic: %v", r)
			time.Sleep(computeTick)
			go computeCollectLoop()
		}
	}()
	time.Sleep(10 * time.Second)
	ticker := time.NewTicker(computeTick)
	defer ticker.Stop()
	for {
		forced := map[uint]bool{}
		select {
		case id := <-computeKickCh:
			forced[id] = true
		drain:
			for {
				select {
				case id := <-computeKickCh:
					forced[id] = true
				default:
					break drain
				}
			}
		case <-ticker.C:
		}
		computeCollectRound(forced)
	}
}

func computeCollectRound(forced map[uint]bool) {
	hosts, err := repo.ComputeHostRepo.List()
	if err != nil || len(hosts) == 0 {
		computeCollectEngines()
		return
	}
	insts, _ := repo.ComputeInstanceRepo.List()
	hasInst := map[uint]bool{}
	for _, in := range insts {
		for _, id := range computeParseIDs(in.HostIDs) {
			hasInst[id] = true
		}
	}
	watching := computeState.watching()
	now := time.Now()
	sem := make(chan struct{}, computeFanout)
	var wg sync.WaitGroup
	slowRan := false
	var slowMu sync.Mutex
	for _, h := range hosts {
		st := computeState.host(h.ID)
		computeState.mu.Lock()
		if st.busy {
			computeState.mu.Unlock()
			continue
		}
		interval := computeFastIdle
		switch {
		case watching:
			interval = computeFastWatch
		case !hasInst[h.ID]:
			interval = computeFastQuiet
		}
		// Back off on unreachable hosts: 15s, 30s, 60s … capped at 5 min.
		if st.fails > 0 {
			back := computeFastIdle << uint(min(st.fails-1, 5))
			if back > computeSlowInterval {
				back = computeSlowInterval
			}
			if back > interval {
				interval = back
			}
		}
		doFast := forced[h.ID] || now.Sub(st.fastAt) >= interval-time.Second
		doSlow := forced[h.ID] || st.slow == nil || now.Sub(st.slowAt) >= computeSlowInterval
		if st.fails > 0 && !forced[h.ID] {
			doSlow = doSlow && doFast
		}
		if !doFast && !doSlow {
			computeState.mu.Unlock()
			continue
		}
		st.busy = true
		computeState.mu.Unlock()

		wg.Add(1)
		sem <- struct{}{}
		go func(h model.ComputeHost, doSlow bool) {
			defer func() {
				if r := recover(); r != nil {
					global.LOG.Errorf("compute collect %s panic: %v", h.Name, r)
				}
				computeState.mu.Lock()
				st.busy = false
				computeState.mu.Unlock()
				<-sem
				wg.Done()
			}()
			if computeCollectHost(h, doSlow) && doSlow {
				slowMu.Lock()
				slowRan = true
				slowMu.Unlock()
			}
		}(h, doSlow)
	}
	wg.Wait()
	if slowRan {
		computeDiscoverTopology(hosts)
	}
	computeCollectEngines()
}

// computeCollectHost runs one combined script and returns true when the
// slow part succeeded.
func computeCollectHost(h model.ComputeHost, doSlow bool) bool {
	script := computeFastScript
	timeout := computeFastTimeout
	if doSlow {
		script = computeSlowScript + computeFastScript
		timeout = computeSlowTimeout
	}
	start := time.Now()
	out, err := computeSSH.RunTimeout(h, script, false, timeout)
	elapsed := time.Since(start)
	st := computeState.host(h.ID)
	now := time.Now()
	if err != nil && !strings.Contains(out, "@@STAT") {
		computeState.mu.Lock()
		st.fails++
		st.fastAt = now
		fails := st.fails
		st.rt.Error = err.Error()
		if fails >= computeOfflineAfter {
			st.rt.Online = false
		}
		computeState.mu.Unlock()
		if fails >= computeOfflineAfter && h.Status != model.ComputeHostOffline {
			_ = repo.ComputeHostRepo.Update(h.ID, map[string]interface{}{"status": model.ComputeHostOffline, "last_error": computeTrim(err.Error(), 500)})
		}
		return false
	}
	fast := parseComputeFast(out)
	var slow *computeSlowRaw
	if doSlow {
		s := parseComputeSlow(out)
		slow = &s
	}

	computeState.mu.Lock()
	prev, prevAt := st.prevFast, st.prevAt
	var addrs map[string]string
	if slow != nil {
		addrs = slow.Addrs
	} else if st.slow != nil {
		addrs = st.slow.Addrs
	}
	rt := computeBuildRealtime(fast, prev, now.Sub(prevAt), addrs)
	rt.CollectMs = elapsed.Milliseconds()
	rt.SampledAt = now.UnixMilli()
	st.rt = rt
	st.prevFast = &fast
	st.prevAt = now
	st.fastAt = now
	st.fails = 0
	if slow != nil {
		st.slow = slow
		st.slowAt = now
	}
	computeState.mu.Unlock()

	vars := map[string]interface{}{}
	if h.Status != model.ComputeHostOnline {
		vars["status"] = model.ComputeHostOnline
		vars["last_error"] = ""
	}
	if slow != nil {
		gpuModel := ""
		unified := false
		if len(fast.GPUs) > 0 {
			gpuModel = fast.GPUs[0].Name
			unified = computeIsUnified(fast.GPUs[0])
		}
		vars["hostname"] = slow.Hostname
		vars["arch"] = slow.Arch
		vars["os"] = slow.OS
		vars["driver"] = slow.Driver
		vars["gpu_model"] = gpuModel
		vars["gpu_count"] = len(fast.GPUs)
		vars["cpu_cores"] = max(slow.Cores, fast.Cores)
		vars["mem_total"] = fast.MemTotal
		vars["unified_mem"] = unified
		vars["asset_sync_at"] = now
		vars["last_seen_at"] = now
	}
	if len(vars) > 0 {
		_ = repo.ComputeHostRepo.Update(h.ID, vars)
	}
	return slow != nil
}

// computeIsUnified: GB10 / GH200-class parts report memory as N/A (or 0)
// because the GPU shares system memory.
func computeIsUnified(g computeGPURaw) bool {
	if g.MemTotal > 0 {
		return false
	}
	name := strings.ToUpper(g.Name)
	return strings.Contains(name, "GB10") || strings.Contains(name, "GH200") || strings.Contains(name, "GB200") || g.MemTotal == 0
}

func computeBuildRealtime(cur computeFastRaw, prev *computeFastRaw, dt time.Duration, addrs map[string]string) dto.ComputeHostRealtime {
	rt := dto.ComputeHostRealtime{Online: true, Load1: cur.Load1, Cores: cur.Cores,
		MemTotal: cur.MemTotal, MemAvailable: cur.MemAvailable, DiskTotal: cur.DiskTotal, DiskUsed: cur.DiskUsed}
	if cur.SwapTotal > cur.SwapFree {
		rt.SwapUsed = cur.SwapTotal - cur.SwapFree
	}
	if cur.MemTotal > 0 {
		rt.MemUsedPercent = computeRound(float64(cur.MemTotal-min(cur.MemAvailable, cur.MemTotal)) / float64(cur.MemTotal) * 100)
	}
	sec := dt.Seconds()
	valid := prev != nil && sec > 0.5 && sec < 900
	if valid && cur.CPU.Total > prev.CPU.Total {
		dTotal := float64(cur.CPU.Total - prev.CPU.Total)
		dIdle := float64(0)
		if cur.CPU.Idle > prev.CPU.Idle {
			dIdle = float64(cur.CPU.Idle - prev.CPU.Idle)
		}
		rt.CpuPercent = computeRound((dTotal - dIdle) / dTotal * 100)
	}
	for _, g := range cur.GPUs {
		item := dto.ComputeGPU{Index: g.Index, Name: g.Name, Util: g.Util, Temp: g.Temp, Power: g.Power,
			MemUsed: uint64(g.MemUsed) << 20, MemTotal: uint64(g.MemTotal) << 20}
		if computeIsUnified(g) {
			item.Unified = true
			item.MemTotal = cur.MemTotal
			item.MemUsed = cur.MemTotal - min(cur.MemAvailable, cur.MemTotal)
		}
		rt.GPUs = append(rt.GPUs, item)
		if g.Temp >= 85 {
			rt.Warnings = append(rt.Warnings, fmt.Sprintf("gpuTemp:%d:%.0f", g.Index, g.Temp))
		}
	}
	prevNic := map[string]computeNicRaw{}
	prevRdma := map[string]computeRdmaRaw{}
	if prev != nil {
		for _, n := range prev.Nics {
			prevNic[n.Name] = n
		}
		for _, r := range prev.Rdma {
			prevRdma[r.Dev+"/"+r.Port] = r
		}
	}
	for _, n := range cur.Nics {
		item := dto.ComputeNic{Name: n.Name, State: n.State, SpeedMbps: n.SpeedMbps, IP: addrs[n.Name]}
		if p, ok := prevNic[n.Name]; ok && valid {
			item.RxRate = computeRate(n.Rx, p.Rx, sec)
			item.TxRate = computeRate(n.Tx, p.Tx, sec)
		}
		rt.Nics = append(rt.Nics, item)
	}
	for _, r := range cur.Rdma {
		item := dto.ComputeRdma{Dev: r.Dev, Port: r.Port, NetDev: r.NetDev, State: r.State, RateGbps: r.RateGbps,
			RcvErrors: r.RcvErrors, SymbolErrors: r.SymbolErrors, LinkDowned: r.LinkDowned,
			XmitDiscards: r.XmitDiscards, LinkErrRecovery: r.LinkErrRecovery}
		if p, ok := prevRdma[r.Dev+"/"+r.Port]; ok && valid {
			// port_*_data counts 4-byte words.
			item.RxRate = computeRate(r.RcvData, p.RcvData, sec) * 4
			item.TxRate = computeRate(r.XmitData, p.XmitData, sec) * 4
			item.ErrDelta = computeDelta(r.RcvErrors, p.RcvErrors) + computeDelta(r.SymbolErrors, p.SymbolErrors) +
				computeDelta(r.LinkDowned, p.LinkDowned) + computeDelta(r.LinkErrRecovery, p.LinkErrRecovery)
			if item.ErrDelta > 0 {
				rt.Warnings = append(rt.Warnings, fmt.Sprintf("rdmaErr:%s:%d", r.Dev, item.ErrDelta))
			}
			if p.State == "ACTIVE" && r.State != "ACTIVE" {
				rt.Warnings = append(rt.Warnings, "rdmaDown:"+r.Dev)
			}
		}
		rt.Rdma = append(rt.Rdma, item)
	}
	if cur.MemTotal > 0 && float64(cur.MemAvailable)/float64(cur.MemTotal) < 0.05 {
		rt.Warnings = append(rt.Warnings, "memLow")
	}
	if rt.SwapUsed > 1<<30 {
		rt.Warnings = append(rt.Warnings, "swap")
	}
	if cur.DiskTotal > 0 && float64(cur.DiskUsed)/float64(cur.DiskTotal) > 0.9 {
		rt.Warnings = append(rt.Warnings, "diskHigh")
	}
	return rt
}

func computeRate(cur, prev uint64, sec float64) float64 {
	if cur < prev || sec <= 0 {
		return 0
	}
	return computeRound(float64(cur-prev) / sec)
}

func computeDelta(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

func computeRound(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func computeTrim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ---- topology discovery ---------------------------------------------------

type computeIfaceRef struct {
	hostID uint
	iface  string
}

// computeDiscoverTopology turns neighbour tables into point-to-point links.
// A neighbour counts as a fabric peer when its IP is configured on another
// registered host and the local interface is not the management interface
// (the one that carries the host's registered address).
func computeDiscoverTopology(hosts []model.ComputeHost) {
	ipOwner := map[string]computeIfaceRef{}
	mgmt := map[uint]string{}
	slows := map[uint]*computeSlowRaw{}
	rdmaOf := map[uint]map[string]dto.ComputeRdma{}
	nicSpeed := map[uint]map[string]int64{}
	for _, h := range hosts {
		slow := computeState.hostSlow(h.ID)
		if slow == nil {
			continue
		}
		slows[h.ID] = slow
		for iface, ip := range slow.Addrs {
			ipOwner[ip] = computeIfaceRef{hostID: h.ID, iface: iface}
			if ip == h.Addr {
				mgmt[h.ID] = iface
			}
		}
		rt := computeState.hostRealtime(h.ID)
		rdmaOf[h.ID] = map[string]dto.ComputeRdma{}
		for _, r := range rt.Rdma {
			if r.NetDev != "" {
				rdmaOf[h.ID][r.NetDev] = r
			}
		}
		nicSpeed[h.ID] = map[string]int64{}
		for _, n := range rt.Nics {
			nicSpeed[h.ID][n.Name] = n.SpeedMbps
		}
	}
	type key struct {
		a, b   uint
		ifaceA string
	}
	found := map[key]model.ComputeFabricLink{}
	for hid, slow := range slows {
		for _, nb := range slow.Neighbours {
			peer, ok := ipOwner[nb.IP]
			if !ok || peer.hostID == hid || nb.Iface == mgmt[hid] || peer.iface == mgmt[peer.hostID] {
				continue
			}
			if strings.HasPrefix(nb.Iface, "docker") || strings.HasPrefix(nb.Iface, "br-") || strings.HasPrefix(nb.Iface, "wl") {
				continue
			}
			l := model.ComputeFabricLink{HostAID: hid, HostBID: peer.hostID, IfaceA: nb.Iface, IfaceB: peer.iface,
				IPA: slow.Addrs[nb.Iface], IPB: nb.IP, Source: "auto"}
			if l.HostAID > l.HostBID {
				l.HostAID, l.HostBID = l.HostBID, l.HostAID
				l.IfaceA, l.IfaceB = l.IfaceB, l.IfaceA
				l.IPA, l.IPB = l.IPB, l.IPA
			}
			if r, ok := rdmaOf[l.HostAID][l.IfaceA]; ok {
				l.RdmaA, l.RateGbps, l.State = r.Dev, r.RateGbps, r.State
			} else if sp := nicSpeed[l.HostAID][l.IfaceA]; sp > 0 {
				l.RateGbps, l.State = int(sp/1000), "ACTIVE"
			}
			if r, ok := rdmaOf[l.HostBID][l.IfaceB]; ok {
				l.RdmaB = r.Dev
				if r.State != "ACTIVE" {
					l.State = r.State
				}
			}
			found[key{l.HostAID, l.HostBID, l.IfaceA}] = l
		}
	}
	if len(found) == 0 {
		return
	}
	existing, _ := repo.ComputeLinkRepo.List()
	now := time.Now()
	for _, l := range found {
		matched := false
		for _, e := range existing {
			if e.HostAID == l.HostAID && e.HostBID == l.HostBID && e.IfaceA == l.IfaceA {
				matched = true
				_ = repo.ComputeLinkRepo.Update(e.ID, map[string]interface{}{"iface_b": l.IfaceB, "ip_a": l.IPA, "ip_b": l.IPB,
					"rdma_a": l.RdmaA, "rdma_b": l.RdmaB, "rate_gbps": l.RateGbps, "state": l.State, "last_seen_at": now})
				break
			}
		}
		if !matched {
			l.LastSeenAt = &now
			_ = repo.ComputeLinkRepo.Create(&l)
		}
	}
	computeRebuildDomains(hosts)
}

// computeRebuildDomains groups links into connected components and names
// each component after its first host (stable as long as the pair holds).
func computeRebuildDomains(hosts []model.ComputeHost) {
	links, err := repo.ComputeLinkRepo.List()
	if err != nil || len(links) == 0 {
		return
	}
	parent := map[uint]uint{}
	var find func(uint) uint
	find = func(x uint) uint {
		if _, ok := parent[x]; !ok {
			parent[x] = x
		}
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	for _, l := range links {
		a, b := find(l.HostAID), find(l.HostBID)
		if a != b {
			parent[max(a, b)] = min(a, b)
		}
	}
	names := map[uint]string{}
	for _, h := range hosts {
		names[h.ID] = h.Name
	}
	members := map[uint][]string{}
	for id := range parent {
		r := find(id)
		members[r] = append(members[r], names[id])
	}
	for _, l := range links {
		m := members[find(l.HostAID)]
		sort.Strings(m)
		domain := "fd-" + m[0]
		if len(m) > 1 {
			domain = fmt.Sprintf("fd-%s+%d", m[0], len(m)-1)
		}
		if l.Domain != domain {
			_ = repo.ComputeLinkRepo.Update(l.ID, map[string]interface{}{"domain": domain})
		}
	}
}

// ---- engine metrics -------------------------------------------------------

var computeEngineBusy sync.Mutex

func computeCollectEngines() {
	if !computeEngineBusy.TryLock() {
		return
	}
	defer computeEngineBusy.Unlock()
	insts, err := repo.ComputeInstanceRepo.List()
	if err != nil || len(insts) == 0 {
		return
	}
	interval := computeFastIdle
	if computeState.watching() {
		interval = computeFastWatch
	}
	runtimes := map[uint]string{}
	profiles, _ := repo.ComputeProfileRepo.List()
	rts, _ := repo.ComputeRuntimeRepo.List()
	rtPath := map[uint]string{}
	for _, r := range rts {
		rtPath[r.ID] = r.MetricsPath
	}
	for _, p := range profiles {
		runtimes[p.ID] = rtPath[p.RuntimeID]
	}
	sem := make(chan struct{}, computeFanout)
	var wg sync.WaitGroup
	now := time.Now()
	for _, in := range insts {
		if in.Status == model.ComputeInstStopped {
			continue
		}
		st := computeState.inst(in.ID)
		computeState.mu.Lock()
		if st.busy || now.Sub(st.prevAt) < interval-time.Second {
			computeState.mu.Unlock()
			continue
		}
		st.busy = true
		computeState.mu.Unlock()
		path := runtimes[in.ProfileID]
		if path == "" {
			path = "/metrics"
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(in model.ComputeInstance, st *computeInstState, path string) {
			defer func() {
				if r := recover(); r != nil {
					global.LOG.Errorf("compute engine %s panic: %v", in.Name, r)
				}
				computeState.mu.Lock()
				st.busy = false
				computeState.mu.Unlock()
				<-sem
				wg.Done()
			}()
			computeCollectEngine(in, st, path)
		}(in, st, path)
	}
	wg.Wait()
}

func computeCollectEngine(in model.ComputeInstance, st *computeInstState, path string) {
	now := time.Now()
	body, err := computeHTTPGet(strings.TrimRight(in.Endpoint, "/")+path, in)
	var m dto.ComputeEngineMetrics
	var sample computeEngineSample
	if err == nil {
		sample = parseComputeMetrics(body, in.ServedName)
		if !sample.Found {
			err = fmt.Errorf("no engine metrics")
		}
	}
	computeState.mu.Lock()
	prev, prevAt := st.prev, st.prevAt
	m = st.m
	if err != nil {
		m.Healthy = false
		m.Error = computeTrim(err.Error(), 300)
		m.SampledAt = now.UnixMilli()
		st.prevAt = now
		computeState.mu.Unlock()
		computeSetInstStatus(in, false, m.Error)
		return
	}
	m = dto.ComputeEngineMetrics{Healthy: true, Running: sample.Running, Waiting: sample.Waiting,
		KVUsage: computeRound(sample.KVUsage), SampledAt: now.UnixMilli(), ReportedModel: sample.ModelName,
		GPUMemUtil: sample.GPUMemUtil, PromptTokens: sample.PromptTokens, GenerationTokens: sample.GenerationTokens,
		LastActiveAt: st.m.LastActiveAt, TTFTMs: st.m.TTFTMs, QueueMs: st.m.QueueMs}
	if prev != nil {
		sec := now.Sub(prevAt).Seconds()
		if sec > 0.5 && sample.GenerationTokens >= prev.GenerationTokens {
			m.GenTPS = computeRound((sample.GenerationTokens - prev.GenerationTokens) / sec)
			m.PromptTPS = computeRound((sample.PromptTokens - prev.PromptTokens) / sec)
		}
		if dc := sample.TTFTCount - prev.TTFTCount; dc > 0 {
			m.TTFTMs = computeRound((sample.TTFTSum - prev.TTFTSum) / dc * 1000)
		}
		if dc := sample.QueueCount - prev.QueueCount; dc > 0 {
			m.QueueMs = computeRound((sample.QueueSum - prev.QueueSum) / dc * 1000)
		}
		if sample.GenerationTokens > prev.GenerationTokens || sample.Running > 0 || sample.Waiting > 0 {
			m.LastActiveAt = now.UnixMilli()
		}
	} else if m.LastActiveAt == 0 {
		// Unknown history: treat the first sample as activity so a freshly
		// observed instance is not scaled down right away.
		m.LastActiveAt = now.UnixMilli()
	}
	if sample.Running > 0 || sample.Waiting > 0 {
		st.idleSince = time.Time{}
	} else if st.idleSince.IsZero() {
		st.idleSince = now
	}
	st.m = m
	s := sample
	st.prev = &s
	st.prevAt = now
	computeState.mu.Unlock()
	computeSetInstStatus(in, true, "")
}

func computeSetInstStatus(in model.ComputeInstance, healthy bool, errMsg string) {
	switch in.Status {
	case model.ComputeInstStarting, model.ComputeInstStopped, model.ComputeInstDraining:
		return // owned by a job
	}
	next := model.ComputeInstRunning
	if !healthy {
		next = model.ComputeInstFailed
	}
	if next != in.Status || (!healthy && in.LastError != errMsg) {
		_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": next, "last_error": errMsg})
	}
}

func computeHTTPGet(url string, in model.ComputeInstance) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), computeMetricTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if key := computeInstanceKey(in); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := computeHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	return string(body), nil
}

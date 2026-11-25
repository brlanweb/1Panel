package service

import (
	"sort"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
)

func (u *ComputeService) Overview() (dto.ComputeOverview, error) {
	var res dto.ComputeOverview
	hosts, err := repo.ComputeHostRepo.List()
	if err != nil {
		return res, err
	}
	insts, _ := repo.ComputeInstanceRepo.List()
	policies, _ := repo.ComputePolicyRepo.List()
	busy := map[uint]bool{}
	for _, in := range insts {
		if in.Status == model.ComputeInstStopped {
			continue
		}
		for _, id := range computeParseIDs(in.HostIDs) {
			busy[id] = true
		}
	}
	gpuUtil, gpuN := 0.0, 0
	for _, h := range hosts {
		res.Hosts++
		rt := computeState.hostRealtime(h.ID)
		if !h.Schedulable {
			res.HostsCordoned++
		}
		if !rt.Online {
			continue
		}
		res.HostsOnline++
		if !busy[h.ID] && h.Schedulable {
			res.HostsIdle++
		}
		res.MemTotal += rt.MemTotal
		res.MemUsed += rt.MemTotal - min(rt.MemAvailable, rt.MemTotal)
		for _, g := range rt.GPUs {
			res.GPUs++
			gpuUtil += g.Util
			gpuN++
		}
		for _, w := range rt.Warnings {
			res.Warnings = append(res.Warnings, h.Name+":"+w)
		}
		for _, r := range rt.Rdma {
			if r.ErrDelta > 0 {
				res.FabricIssues++
			}
		}
	}
	if gpuN > 0 {
		res.GPUUtilAvg = computeRound(gpuUtil / float64(gpuN))
	}
	models := map[string]*dto.ComputeModelSummary{}
	for _, p := range policies {
		models[p.ServedName] = &dto.ComputeModelSummary{ServedName: p.ServedName, PolicyMode: p.Mode,
			MinInstances: p.MinInstances, MaxInstances: p.MaxInstances}
	}
	ttftW := map[string]float64{}
	for _, in := range insts {
		res.Instances++
		m := computeState.instMetrics(in.ID)
		s := models[in.ServedName]
		if s == nil {
			s = &dto.ComputeModelSummary{ServedName: in.ServedName, PolicyMode: "off"}
			models[in.ServedName] = s
		}
		s.Instances++
		if m.Healthy && in.Status == model.ComputeInstRunning {
			res.InstHealthy++
			s.Healthy++
			s.Running += m.Running
			s.Waiting += m.Waiting
			s.GenTPS += m.GenTPS
			res.GenTPS += m.GenTPS
			if m.KVUsage > s.KVUsage {
				s.KVUsage = m.KVUsage
			}
			if m.TTFTMs > 0 {
				w := m.Running + 1
				s.TTFTMs += m.TTFTMs * w
				ttftW[in.ServedName] += w
			}
		}
	}
	for name, s := range models {
		if w := ttftW[name]; w > 0 {
			s.TTFTMs = computeRound(s.TTFTMs / w)
		}
		s.GenTPS = computeRound(s.GenTPS)
		res.Models = append(res.Models, *s)
	}
	sort.Slice(res.Models, func(i, j int) bool { return res.Models[i].ServedName < res.Models[j].ServedName })
	res.GenTPS = computeRound(res.GenTPS)
	domains, _ := u.Topology()
	res.Domains = len(domains)
	res.RunningJobs = int(computeCount(repo.ComputeJobRepo.Count(repo.WithComputeField("status", model.ComputeJobRunning))))
	res.Suggestions = len(u.Suggestions())
	res.Router, _ = u.GetRouterSetting()
	return res, nil
}

func computeCount(n int64, _ error) int64 { return n }

// computeSeedRuntimes inserts the built-in runtimes once (by name).
func computeSeedRuntimes() {
	for _, rt := range computeBuiltinRuntimes() {
		if _, err := repo.ComputeRuntimeRepo.Get(repo.WithByName(rt.Name)); err == nil {
			continue
		}
		item := rt
		_ = repo.ComputeRuntimeRepo.Create(&item)
	}
}

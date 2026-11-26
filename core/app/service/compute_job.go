package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
)

// Jobs: deploy / switch / stop / start / restart / remove.
//
// Every job runs in its own goroutine, persists its step list after each
// step and holds a per-host lock, so two jobs never touch the same host at
// the same time. Switch = drain → stop old → start new → wait healthy →
// route; on failure it restarts the previous profile (rollback).

const (
	computeDrainMax    = 120 * time.Second
	computeStepTimeout = 10 * time.Minute
	computeHealthPoll  = 10 * time.Second
	computeMemHeadroom = 4 << 30
)

var computeHostLocks sync.Map // hostID -> *sync.Mutex (TryLock)

type computeJobSpec struct {
	Type       string
	InstanceID uint
	ProfileID  uint
	ServedName string
	HostIDs    []uint
	Source     string
	Reason     string
	Name       string
	// Port of a new replica (several replicas may share a host).
	Port int
}

type computeJobRun struct {
	job     model.ComputeJob
	steps   []dto.ComputeJobStep
	savedAt time.Time
}

func (r *computeJobRun) step(name string, fn func() (string, error)) error {
	s := dto.ComputeJobStep{Name: name, Status: "running", StartedAt: time.Now().UnixMilli()}
	r.steps = append(r.steps, s)
	r.save()
	msg, err := fn()
	i := len(r.steps) - 1
	r.steps[i].FinishedAt = time.Now().UnixMilli()
	r.steps[i].Message = computeTrim(msg, 2000)
	if err != nil {
		r.steps[i].Status = "failed"
		if r.steps[i].Message == "" {
			r.steps[i].Message = err.Error()
		} else {
			r.steps[i].Message = computeTrim(err.Error()+"\n"+r.steps[i].Message, 2000)
		}
	} else {
		r.steps[i].Status = "success"
	}
	r.save()
	return err
}

// progress replaces the message of the running step (throttled), for long
// steps such as a model copy.
func (r *computeJobRun) progress(msg string) {
	if len(r.steps) == 0 {
		return
	}
	i := len(r.steps) - 1
	r.steps[i].Message = computeTrim(msg, 2000)
	if time.Since(r.savedAt) < 3*time.Second {
		return
	}
	r.save()
}

func (r *computeJobRun) save() {
	r.savedAt = time.Now()
	b, _ := json.Marshal(r.steps)
	_ = repo.ComputeJobRepo.Update(r.job.ID, map[string]interface{}{"steps": string(b)})
}

func (r *computeJobRun) finish(status string, err error) {
	now := time.Now()
	vars := map[string]interface{}{"status": status, "finished_at": now}
	if err != nil {
		vars["error"] = computeTrim(err.Error(), 2000)
	}
	_ = repo.ComputeJobRepo.Update(r.job.ID, vars)
}

func computeLockHosts(ids []uint) (func(), bool) {
	var locked []*sync.Mutex
	for _, id := range ids {
		v, _ := computeHostLocks.LoadOrStore(id, &sync.Mutex{})
		m := v.(*sync.Mutex)
		if !m.TryLock() {
			for _, l := range locked {
				l.Unlock()
			}
			return nil, false
		}
		locked = append(locked, m)
	}
	return func() {
		for _, l := range locked {
			l.Unlock()
		}
	}, true
}

func computeStartJob(spec computeJobSpec) (uint, error) {
	unlock, ok := computeLockHosts(spec.HostIDs)
	if !ok {
		return 0, buserr.New("ErrComputeHostBusy")
	}
	now := time.Now()
	job := model.ComputeJob{Type: spec.Type, Status: model.ComputeJobRunning, ServedName: spec.ServedName,
		InstanceID: spec.InstanceID, ProfileID: spec.ProfileID, HostIDs: computeJoinIDs(spec.HostIDs),
		Source: spec.Source, Reason: spec.Reason, Steps: "[]", StartedAt: &now}
	if err := repo.ComputeJobRepo.Create(&job); err != nil {
		unlock()
		return 0, err
	}
	run := &computeJobRun{job: job}
	go func() {
		defer unlock()
		defer func() {
			if r := recover(); r != nil {
				global.LOG.Errorf("compute job %d panic: %v", job.ID, r)
				run.finish(model.ComputeJobFailed, fmt.Errorf("panic: %v", r))
			}
		}()
		var err error
		status := model.ComputeJobSuccess
		switch spec.Type {
		case "deploy":
			err = computeRunDeploy(run, spec)
		case "switch":
			var rolled bool
			rolled, err = computeRunSwitch(run, spec)
			if rolled {
				status = model.ComputeJobRolledBack
			}
		case "stop", "remove", "scaleDown":
			err = computeRunStop(run, spec, spec.Type != "stop")
		case "start", "restart":
			err = computeRunStart(run, spec)
		default:
			err = fmt.Errorf("unknown job type %s", spec.Type)
		}
		if err != nil && status != model.ComputeJobRolledBack {
			status = model.ComputeJobFailed
		}
		run.finish(status, err)
		for _, id := range spec.HostIDs {
			computeKick(id)
		}
	}()
	return job.ID, nil
}

// ---- precheck / placement -------------------------------------------------

// computeHostUse is what already runs on a host.
type computeHostUse struct {
	insts []string
	multi bool // part of a multi-host group
	ports map[int]bool
}

func computeHostUsage(excludeInst uint) map[uint]*computeHostUse {
	insts, _ := repo.ComputeInstanceRepo.List()
	res := map[uint]*computeHostUse{}
	for _, in := range insts {
		if in.ID == excludeInst || in.Status == model.ComputeInstStopped {
			continue
		}
		ids := computeParseIDs(in.HostIDs)
		for _, id := range ids {
			u := res[id]
			if u == nil {
				u = &computeHostUse{ports: map[int]bool{}}
				res[id] = u
			}
			u.insts = append(u.insts, in.Name)
			u.ports[in.Port] = true
			if len(ids) > 1 {
				u.multi = true
			}
		}
	}
	return res
}

// computeHostFits: a host takes another replica of the plan when it is
// empty, or (single-host plans with MaxPerHost > 1) it only runs
// single-host replicas and fewer than MaxPerHost of them.
func computeHostFits(prof model.ComputeProfile, u *computeHostUse) bool {
	if u == nil || len(u.insts) == 0 {
		return true
	}
	if prof.NodeCount > 1 || prof.MaxPerHost <= 1 || u.multi {
		return false
	}
	return len(u.insts) < prof.MaxPerHost
}

// computePickPort returns the first port from the plan's port on that is
// free on every host (instances and listening engine processes).
func computePickPort(prof model.ComputeProfile, ids []uint, usage map[uint]*computeHostUse) int {
	base := prof.Port
	if base <= 0 {
		base = 8000
	}
	for p := base; p < base+200 && p <= 65535; p++ {
		ok := true
		for _, id := range ids {
			if u := usage[id]; u != nil && u.ports[p] {
				ok = false
				break
			}
			if slow := computeState.hostSlow(id); slow != nil {
				for _, proc := range slow.Procs {
					if proc.Port == p {
						ok = false
					}
				}
			}
		}
		if ok {
			return p
		}
	}
	return 0
}

func computeProfileHosts(prof model.ComputeProfile) []uint { return computeParseIDs(prof.HostIDs) }

func computePrecheck(prof model.ComputeProfile, rt model.ComputeRuntime, hostIDs []uint, excludeInst uint) dto.ComputePrecheckRes {
	res := dto.ComputePrecheckRes{OK: true}
	add := func(name string, ok bool, msg string) {
		res.Items = append(res.Items, dto.ComputePrecheckItem{Name: name, OK: ok, Message: msg})
		if !ok {
			res.OK = false
		}
	}
	hosts := computeHostMap()
	usage := computeHostUsage(excludeInst)
	if len(hostIDs) == 0 {
		if pinned := computeProfileHosts(prof); prof.NodeCount > 1 && len(pinned) > 0 {
			hostIDs = pinned
			add("placement", true, strings.Join(computeHostNames(hostIDs, hosts), ", "))
		} else {
			hostIDs = computePlace(prof, rt, hosts, usage)
			if len(hostIDs) == 0 {
				add("placement", false, fmt.Sprintf("no %d host(s) with room for the plan", max(prof.NodeCount, 1)))
				return res
			}
			add("placement", true, strings.Join(computeHostNames(hostIDs, hosts), ", "))
		}
	}
	res.HostIDs = hostIDs
	add("nodeCount", len(hostIDs) == max(prof.NodeCount, 1), fmt.Sprintf("%d / %d", len(hostIDs), max(prof.NodeCount, 1)))
	if prof.NodeCount > 1 && !rt.MultiNode {
		add("runtime", false, "runtime "+rt.Name+" is single node")
	}
	if rt.Backend == model.ComputeBackendVllm {
		for _, id := range hostIDs {
			if h, ok := hosts[id]; ok && !h.IsPanel() {
				add(h.Name+":backend", false, "runtime "+rt.Name+" deploys through AI → vLLM and needs a panel host (cluster node / local)")
			}
		}
	}
	res.Port = computePickPort(prof, hostIDs, usage)
	if res.Port == 0 {
		add("port", false, fmt.Sprintf("no free port from %d on", prof.Port))
	} else {
		add("port", true, fmt.Sprintf("%d", res.Port))
	}
	for _, id := range hostIDs {
		h, ok := hosts[id]
		if !ok {
			add("host", false, fmt.Sprintf("#%d not found", id))
			continue
		}
		rtm := computeState.hostRealtime(id)
		add(h.Name+":online", rtm.Online, rtm.Error)
		if !h.Schedulable {
			add(h.Name+":schedulable", false, "cordoned")
		}
		if u := usage[id]; !computeHostFits(prof, u) {
			msg := "used by " + strings.Join(u.insts, ", ")
			if prof.NodeCount <= 1 && prof.MaxPerHost > 1 && !u.multi {
				msg += fmt.Sprintf(" (%d / %d replicas per host)", len(u.insts), prof.MaxPerHost)
			}
			add(h.Name+":free", false, msg)
		}
		if prof.MinMemGB > 0 && rtm.Online {
			need := uint64(prof.MinMemGB) << 30
			// The old instance of a switch frees its memory first.
			avail, pool := computeFreeMem(rtm, rt.Device, excludeInst > 0 && usage[id] == nil)
			add(h.Name+":memory", avail+computeMemHeadroom >= need,
				fmt.Sprintf("%.1f GiB %s available / %d GiB required", float64(avail)/(1<<30), pool, prof.MinMemGB))
		}
		if rtm.Online {
			if n := computeAccelCount(rtm, rt.Device); n == 0 {
				add(h.Name+":"+computeDeviceKind(rt.Device), false, "no "+strings.ToUpper(computeDeviceKind(rt.Device))+" detected")
			}
		}
	}
	if prof.NeedFabric && len(hostIDs) > 1 {
		fab := computeFabricOf(hostIDs)
		ok := len(fab) == len(hostIDs) && computeConnected(hostIDs)
		add("fabric", ok, "hosts must share one fabric domain with ACTIVE links")
	}
	msg, ok := computeModelCheck(prof, hostIDs, hosts)
	add("model", ok, msg)
	return res
}

// computeModelCheck: every host has the model, or it can be copied there
// from a host that has it.
func computeModelCheck(prof model.ComputeProfile, ids []uint, hosts map[uint]model.ComputeHost) (string, bool) {
	place := computeLocateModel(prof, ids, hosts)
	if len(place.Missing) == 0 {
		var parts []string
		for _, id := range ids {
			parts = append(parts, hosts[id].Name+": "+place.Have[id])
		}
		return strings.Join(parts, "; "), true
	}
	src, dir, ok := computeModelSource(prof, ids, place.Missing, hosts, place)
	if !ok {
		return computeNoModelErr(prof, place).Error(), false
	}
	return fmt.Sprintf("copy %s from %s to %s before start", dir, hosts[src].Name,
		strings.Join(computeHostNames(place.Missing, hosts), ", ")), true
}

func computeConnected(ids []uint) bool {
	links, _ := computeLinks()
	member := map[uint]bool{}
	for _, id := range ids {
		member[id] = true
	}
	adj := map[uint][]uint{}
	for _, l := range links {
		if member[l.HostAID] && member[l.HostBID] && l.State == "ACTIVE" {
			adj[l.HostAID] = append(adj[l.HostAID], l.HostBID)
			adj[l.HostBID] = append(adj[l.HostBID], l.HostAID)
		}
	}
	seen := map[uint]bool{ids[0]: true}
	queue := []uint{ids[0]}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		for _, y := range adj[x] {
			if !seen[y] {
				seen[y] = true
				queue = append(queue, y)
			}
		}
	}
	return len(seen) == len(ids)
}

// computePlace picks hosts for a profile: online, schedulable, with room
// (free, or below MaxPerHost single-host replicas) and enough memory.
// Plans pinned to hosts only use those; a pinned multi-host plan uses
// exactly its hosts. Replicas spread first (fewest replicas, then most
// free memory).
func computePlace(prof model.ComputeProfile, runtime model.ComputeRuntime, hosts map[uint]model.ComputeHost, usage map[uint]*computeHostUse) []uint {
	pinned := computeProfileHosts(prof)
	eligible := func(id uint) (uint64, bool) {
		h, ok := hosts[id]
		if !ok || !h.Schedulable || !computeHostFits(prof, usage[id]) {
			return 0, false
		}
		if runtime.Backend == model.ComputeBackendVllm && !h.IsPanel() {
			return 0, false
		}
		rt := computeState.hostRealtime(id)
		if !rt.Online || computeAccelCount(rt, runtime.Device) == 0 {
			return 0, false
		}
		mem, _ := computeFreeMem(rt, runtime.Device, false)
		if prof.MinMemGB > 0 && mem+computeMemHeadroom < uint64(prof.MinMemGB)<<30 {
			return 0, false
		}
		return mem, true
	}
	if prof.NodeCount > 1 && len(pinned) > 0 {
		for _, id := range pinned {
			if _, ok := eligible(id); !ok {
				return nil
			}
		}
		return pinned
	}
	cands := pinned
	if len(cands) == 0 {
		cands = computeAllHostIDs(hosts)
	}
	var free []uint
	avail := map[uint]uint64{}
	for _, id := range cands {
		if mem, ok := eligible(id); ok {
			avail[id] = mem
			free = append(free, id)
		}
	}
	load := func(id uint) int {
		if u := usage[id]; u != nil {
			return len(u.insts)
		}
		return 0
	}
	sort.Slice(free, func(i, j int) bool {
		if load(free[i]) != load(free[j]) {
			return load(free[i]) < load(free[j])
		}
		if avail[free[i]] != avail[free[j]] {
			return avail[free[i]] > avail[free[j]]
		}
		return free[i] < free[j]
	})
	if prof.NodeCount <= 1 {
		if len(free) == 0 {
			return nil
		}
		return free[:1]
	}
	if !prof.NeedFabric {
		if len(free) < prof.NodeCount {
			return nil
		}
		return free[:prof.NodeCount]
	}
	freeSet := map[uint]bool{}
	for _, id := range free {
		freeSet[id] = true
	}
	domains, _ := (&ComputeService{}).Topology()
	for _, d := range domains {
		if !d.Healthy {
			continue
		}
		var cand []uint
		for _, id := range d.HostIDs {
			if freeSet[id] {
				cand = append(cand, id)
			}
		}
		if len(cand) >= prof.NodeCount && computeConnected(cand[:prof.NodeCount]) {
			return cand[:prof.NodeCount]
		}
	}
	return nil
}

// computeCanPlace reports whether one more replica of the plan fits.
func computeCanPlace(prof model.ComputeProfile) bool {
	rt, err := repo.ComputeRuntimeRepo.Get(repo.WithByID(prof.RuntimeID))
	if err != nil {
		return false
	}
	return len(computePlace(prof, rt, computeHostMap(), computeHostUsage(0))) > 0
}

// computeReplicaName is "<model>@<head host>", numbered when the host
// already runs a replica with that name.
func computeReplicaName(served, host string) string {
	base := fmt.Sprintf("%s@%s", served, host)
	name := base
	for i := 2; i < 100; i++ {
		if _, err := repo.ComputeInstanceRepo.Get(repo.WithByName(name)); err != nil {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix()%100000)
}

// ---- public entry points --------------------------------------------------

func (u *ComputeService) Deploy(req dto.ComputeDeployReq) (dto.ComputePrecheckRes, error) {
	prof, err := repo.ComputeProfileRepo.Get(repo.WithByID(req.ProfileID))
	if err != nil {
		return dto.ComputePrecheckRes{}, buserr.New("ErrRecordNotFound")
	}
	rt, err := repo.ComputeRuntimeRepo.Get(repo.WithByID(prof.RuntimeID))
	if err != nil {
		return dto.ComputePrecheckRes{}, buserr.New("ErrRecordNotFound")
	}
	res := computePrecheck(prof, rt, req.HostIDs, 0)
	if req.DryRun || !res.OK {
		return res, nil
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		hosts := computeHostMap()
		name = computeReplicaName(prof.ServedName, hosts[res.HostIDs[0]].Name)
	}
	if _, err := repo.ComputeInstanceRepo.Get(repo.WithByName(name)); err == nil {
		return res, buserr.New("ErrRecordExist")
	}
	id, err := computeStartJob(computeJobSpec{Type: "deploy", ProfileID: prof.ID, ServedName: prof.ServedName,
		HostIDs: res.HostIDs, Source: "manual", Name: name, Port: res.Port})
	res.JobID = id
	return res, err
}

func (u *ComputeService) Switch(req dto.ComputeSwitchReq) (dto.ComputePrecheckRes, error) {
	in, err := repo.ComputeInstanceRepo.Get(repo.WithByID(req.InstanceID))
	if err != nil {
		return dto.ComputePrecheckRes{}, buserr.New("ErrRecordNotFound")
	}
	prof, err := repo.ComputeProfileRepo.Get(repo.WithByID(req.ProfileID))
	if err != nil {
		return dto.ComputePrecheckRes{}, buserr.New("ErrRecordNotFound")
	}
	rt, err := repo.ComputeRuntimeRepo.Get(repo.WithByID(prof.RuntimeID))
	if err != nil {
		return dto.ComputePrecheckRes{}, buserr.New("ErrRecordNotFound")
	}
	ids := computeParseIDs(in.HostIDs)
	if len(ids) != max(prof.NodeCount, 1) {
		return dto.ComputePrecheckRes{Items: []dto.ComputePrecheckItem{{Name: "nodeCount", Message: fmt.Sprintf("instance has %d host(s), profile needs %d", len(ids), prof.NodeCount)}}}, nil
	}
	res := computePrecheck(prof, rt, ids, in.ID)
	if in.Adopted && in.ContainerName == "" {
		res.OK = false
		res.Items = append(res.Items, dto.ComputePrecheckItem{Name: "adopted", Message: "adopted instance without container name cannot be stopped safely"})
	}
	if req.DryRun || !res.OK {
		return res, nil
	}
	id, err := computeStartJob(computeJobSpec{Type: "switch", InstanceID: in.ID, ProfileID: prof.ID,
		ServedName: prof.ServedName, HostIDs: ids, Source: "manual", Port: res.Port})
	res.JobID = id
	return res, err
}

func (u *ComputeService) PageJobs(req dto.SearchWithPage) (int64, []model.ComputeJob, error) {
	return repo.ComputeJobRepo.Page(req.Page, req.PageSize, repo.WithComputeLike([]string{"served_name", "type", "status"}, req.Info), repo.WithComputeOrder("id desc"))
}

func (u *ComputeService) GetJob(id uint) (model.ComputeJob, error) {
	return repo.ComputeJobRepo.Get(repo.WithByID(id))
}

// ---- job bodies -----------------------------------------------------------

func computeLoadSpec(profileID uint) (model.ComputeProfile, model.ComputeRuntime, error) {
	prof, err := repo.ComputeProfileRepo.Get(repo.WithByID(profileID))
	if err != nil {
		return prof, model.ComputeRuntime{}, fmt.Errorf("profile #%d not found", profileID)
	}
	rt, err := repo.ComputeRuntimeRepo.Get(repo.WithByID(prof.RuntimeID))
	if err != nil {
		return prof, rt, fmt.Errorf("runtime #%d not found", prof.RuntimeID)
	}
	return prof, rt, nil
}

func computeRunDeploy(run *computeJobRun, spec computeJobSpec) error {
	prof, rt, err := computeLoadSpec(spec.ProfileID)
	if err != nil {
		return err
	}
	hosts := computeHostMap()
	head := hosts[spec.HostIDs[0]]
	key := computeRandomKey("sk-node-")
	keyCipher, _ := computeEncrypt(key)
	port := spec.Port
	if port <= 0 {
		port = prof.Port
	}
	in := model.ComputeInstance{Name: spec.Name, ProfileID: prof.ID, ServedName: prof.ServedName, HeadHostID: head.ID,
		HostIDs: computeJoinIDs(spec.HostIDs), Port: port, Endpoint: computeEndpoint(head.Addr, port),
		APIKeyCipher: keyCipher, ContainerName: computeContainerName(spec.Name), Status: model.ComputeInstStarting,
		RouteEnabled: false, Weight: 100}
	if err := repo.ComputeInstanceRepo.Create(&in); err != nil {
		return err
	}
	_ = repo.ComputeJobRepo.Update(run.job.ID, map[string]interface{}{"instance_id": in.ID})
	if err := computeLaunch(run, in, prof, rt, spec.HostIDs, hosts, key); err != nil {
		_ = run.step("cleanup", func() (string, error) { return computeStopHosts(in, rt, spec.HostIDs, hosts) })
		_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstFailed, "last_error": computeTrim(err.Error(), 500)})
		return err
	}
	return repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstRunning, "route_enabled": true, "last_error": ""})
}

// computeLaunch: model on every host → render → start workers → start
// head → wait healthy → availability test. The instance only takes traffic
// (route_enabled, set by the caller) after the test passed.
func computeLaunch(run *computeJobRun, in model.ComputeInstance, prof model.ComputeProfile, rt model.ComputeRuntime,
	ids []uint, hosts map[uint]model.ComputeHost, key string) error {
	dir, err := computeEnsureModel(run, prof, ids, hosts)
	if err != nil {
		_ = run.step("model", func() (string, error) { return "", err })
		return err
	}
	prof.ModelPath = dir
	prof = computeTuneProfile(prof, rt, ids)
	if prof.Image != "" {
		rt.Image = prof.Image
	}
	if rt.Backend == model.ComputeBackendVllm {
		if len(ids) != 1 {
			return fmt.Errorf("runtime %s is single host", rt.Name)
		}
		h := hosts[ids[0]]
		if err := run.step(fmt.Sprintf("deploy on %s (AI → vLLM)", h.Name), func() (string, error) {
			depID, ctr, msg, err := computeVllmLaunch(in, prof, rt, h, key)
			if depID > 0 {
				_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"vllm_deployment_id": depID, "container_name": ctr})
			}
			return msg, err
		}); err != nil {
			return err
		}
	} else {
		if in.VllmDeploymentID > 0 {
			_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"vllm_deployment_id": 0})
		}
		if err := computeLaunchScripts(run, in, prof, rt, ids, hosts, key); err != nil {
			return err
		}
	}
	timeout := time.Duration(prof.StartTimeoutSec) * time.Second
	if err := run.step("wait healthy", func() (string, error) {
		return computeWaitHealthy(in, rt, key, timeout)
	}); err != nil {
		return err
	}
	return run.step("availability test", func() (string, error) { return computeVerify(in, key) })
}

func computeLaunchScripts(run *computeJobRun, in model.ComputeInstance, prof model.ComputeProfile, rt model.ComputeRuntime,
	ids []uint, hosts map[uint]model.ComputeHost, key string) error {
	vars, err := computeBuildVars(in, prof, rt, ids, hosts, key)
	if err != nil {
		return err
	}
	scripts := make([]string, len(vars))
	for i, v := range vars {
		s, err := computeRender(rt.Name, rt.LaunchTemplate, v)
		if err != nil {
			return err
		}
		scripts[i] = s
	}
	// Workers first (they wait for the head), head last.
	order := make([]int, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		order = append(order, i)
	}
	for _, i := range order {
		h := hosts[ids[i]]
		role := "worker"
		if i == 0 {
			role = "head"
		}
		script := scripts[i]
		if err := run.step(fmt.Sprintf("start %s (%s)", h.Name, role), func() (string, error) {
			return computeExec(h, script, true, computeStepTimeout)
		}); err != nil {
			return err
		}
	}
	return nil
}

func computeWaitHealthy(in model.ComputeInstance, rt model.ComputeRuntime, key string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(in.Endpoint, "/") + rt.HealthPath
	last := ""
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := computeHTTP.Do(req)
		cancel()
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return "healthy", nil
			}
			last = fmt.Sprintf("http %d", resp.StatusCode)
		} else {
			last = err.Error()
		}
		time.Sleep(computeHealthPoll)
	}
	return last, fmt.Errorf("not healthy after %s", timeout)
}

// computeVerify sends a real chat completion through the instance's own
// endpoint and key: HTTP 200 with a non-empty answer is required before the
// instance joins its pool. The result reports first-token latency.
func computeVerify(in model.ComputeInstance, key string) (string, error) {
	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"Reply with the single word: ok"}],"max_tokens":16,"temperature":0,"stream":true}`,
		computeUpstreamModel(in))
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		msg, err := computeVerifyOnce(in, key, body)
		if err == nil {
			return msg, nil
		}
		last = err
		time.Sleep(time.Duration(attempt) * 5 * time.Second)
	}
	return "", fmt.Errorf("model test failed: %v", last)
}

func computeVerifyOnce(in model.ComputeInstance, key, body string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(in.Endpoint, "/")+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	start := time.Now()
	resp, err := computeUpstream.RoundTrip(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("http %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var ttft time.Duration
	var text strings.Builder
	reader := bufio.NewReader(resp.Body)
	for {
		line, rerr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content"`
						Reasoning        string `json:"reasoning"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(data), &chunk) == nil {
				for _, c := range chunk.Choices {
					piece := c.Delta.Content + c.Delta.ReasoningContent + c.Delta.Reasoning
					if piece != "" && ttft == 0 {
						ttft = time.Since(start)
					}
					text.WriteString(piece)
				}
			}
		}
		if rerr != nil {
			break
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", fmt.Errorf("empty answer")
	}
	return fmt.Sprintf("ok · first token %s · total %s · %q", ttft.Round(time.Millisecond), time.Since(start).Round(time.Millisecond),
		computeTrim(strings.TrimSpace(text.String()), 60)), nil
}

// computeTuneProfile adjusts a plan for the actual hosts: the image of the
// plan overrides the runtime's, and with several replicas per host every
// engine is capped to its share of accelerator memory (MinMemGB of the
// total), so the replicas fit next to each other.
func computeTuneProfile(prof model.ComputeProfile, rt model.ComputeRuntime, ids []uint) model.ComputeProfile {
	if prof.NodeCount > 1 || prof.MaxPerHost <= 1 || prof.MinMemGB <= 0 || len(ids) == 0 {
		return prof
	}
	if strings.Contains(prof.ExtraArgs, "--gpu-memory-utilization") {
		return prof
	}
	total, _ := computeFreeMem(computeState.hostRealtime(ids[0]), rt.Device, true)
	if total == 0 {
		return prof
	}
	util := float64(prof.MinMemGB) * (1 << 30) / float64(total)
	if util > 0.9 {
		return prof
	}
	util = math.Max(0.05, math.Ceil(util*100)/100)
	arg := fmt.Sprintf("--gpu-memory-utilization %.2f", util)
	if strings.TrimSpace(prof.ExtraArgs) == "" {
		prof.ExtraArgs = arg
	} else if rt.Backend == model.ComputeBackendVllm {
		prof.ExtraArgs = strings.TrimRight(prof.ExtraArgs, "\n") + "\n" + arg
	} else {
		prof.ExtraArgs = strings.TrimSpace(prof.ExtraArgs) + " " + arg
	}
	return prof
}

func computeStopHosts(in model.ComputeInstance, rt model.ComputeRuntime, ids []uint, hosts map[uint]model.ComputeHost) (string, error) {
	return computeStopHostsMode(in, rt, ids, hosts, false)
}

// computeStopHostsMode stops the instance; remove also deletes the AI →
// vLLM deployment of a vllm-backed instance (scripts always remove the
// container).
func computeStopHostsMode(in model.ComputeInstance, rt model.ComputeRuntime, ids []uint, hosts map[uint]model.ComputeHost, remove bool) (string, error) {
	if in.VllmDeploymentID > 0 || rt.Backend == model.ComputeBackendVllm {
		if len(ids) == 0 {
			return "", nil
		}
		return computeVllmOperate(in, hosts[ids[0]], remove)
	}
	var msgs []string
	var firstErr error
	for i, id := range ids {
		h := hosts[id]
		script, err := computeRender("stop", rt.StopTemplate, computeTplVars{Name: in.Name, ContainerName: in.ContainerName,
			Port: in.Port, Env: map[string]string{}, Rank: i, IsHead: i == 0, NodeCount: len(ids)})
		if err != nil {
			return "", err
		}
		out, err := computeExec(h, script, true, 2*time.Minute)
		msgs = append(msgs, h.Name+": "+strings.TrimSpace(out))
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", h.Name, err)
		}
	}
	return strings.Join(msgs, "\n"), firstErr
}

// computeDrain removes the instance from routing and waits until the
// router has no request in flight to it (bounded).
func computeDrain(run *computeJobRun, in model.ComputeInstance) {
	_ = run.step("drain", func() (string, error) {
		_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"route_enabled": false, "status": model.ComputeInstDraining})
		computeRouterInvalidate()
		st := computeState.inst(in.ID)
		deadline := time.Now().Add(computeDrainMax)
		for time.Now().Before(deadline) {
			if st.inflight.Load() == 0 {
				return "drained", nil
			}
			time.Sleep(2 * time.Second)
		}
		return fmt.Sprintf("%d request(s) still in flight, continuing", st.inflight.Load()), nil
	})
}

func computeRunSwitch(run *computeJobRun, spec computeJobSpec) (bool, error) {
	in, err := repo.ComputeInstanceRepo.Get(repo.WithByID(spec.InstanceID))
	if err != nil {
		return false, err
	}
	newProf, newRt, err := computeLoadSpec(spec.ProfileID)
	if err != nil {
		return false, err
	}
	oldProfID := in.ProfileID
	var oldRt model.ComputeRuntime
	if oldProfID > 0 {
		_, oldRt, _ = computeLoadSpec(oldProfID)
	}
	if oldRt.ID == 0 {
		// Adopted instance: stop its container with the generic stop script.
		oldRt = model.ComputeRuntime{StopTemplate: computeStopDocker}
	}
	hosts := computeHostMap()
	ids := spec.HostIDs
	computeDrain(run, in)
	leaveVllm := in.VllmDeploymentID > 0 && newRt.Backend != model.ComputeBackendVllm
	if err := run.step("stop old", func() (string, error) { return computeStopHostsMode(in, oldRt, ids, hosts, leaveVllm) }); err != nil {
		return false, err
	}
	if leaveVllm {
		in.VllmDeploymentID = 0
	}
	key := computeInstanceKey(in)
	if key == "" {
		key = computeRandomKey("sk-node-")
	}
	keyCipher, _ := computeEncrypt(key)
	next := in
	port := spec.Port
	if port <= 0 {
		port = newProf.Port
	}
	next.ProfileID, next.ServedName, next.Port = newProf.ID, newProf.ServedName, port
	// The panel launches the new engine with --served-model-name set to the
	// profile name, so an alias of the adopted engine no longer applies.
	next.UpstreamModel = ""
	next.Endpoint = computeEndpoint(hosts[ids[0]].Addr, port)
	if next.ContainerName == "" {
		next.ContainerName = computeContainerName(in.Name)
	}
	_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstStarting, "profile_id": next.ProfileID,
		"served_name": next.ServedName, "upstream_model": "", "port": next.Port, "endpoint": next.Endpoint,
		"container_name": next.ContainerName, "api_key_cipher": keyCipher, "adopted": false})
	computeRouterInvalidate()
	computeState.dropInst(in.ID)
	if err := computeLaunch(run, next, newProf, newRt, ids, hosts, key); err != nil {
		_ = run.step("stop new", func() (string, error) { return computeStopHosts(next, newRt, ids, hosts) })
		if oldProfID == 0 {
			_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstFailed, "last_error": computeTrim(err.Error(), 500)})
			return false, err
		}
		oldProf, oldRt2, lerr := computeLoadSpec(oldProfID)
		if lerr != nil {
			return false, err
		}
		_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"profile_id": in.ProfileID, "served_name": in.ServedName,
			"upstream_model": in.UpstreamModel, "port": in.Port, "endpoint": in.Endpoint})
		computeRouterInvalidate()
		rb := in
		rb.ContainerName = next.ContainerName
		if rerr := computeLaunch(run, rb, oldProf, oldRt2, ids, hosts, key); rerr != nil {
			_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstFailed, "last_error": computeTrim(rerr.Error(), 500)})
			return false, fmt.Errorf("switch failed: %v; rollback failed: %v", err, rerr)
		}
		_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstRunning, "route_enabled": true})
		computeRouterInvalidate()
		return true, fmt.Errorf("switch failed, rolled back: %v", err)
	}
	_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstRunning, "route_enabled": true, "last_error": ""})
	computeRouterInvalidate()
	return false, nil
}

func computeRunStop(run *computeJobRun, spec computeJobSpec, remove bool) error {
	in, err := repo.ComputeInstanceRepo.Get(repo.WithByID(spec.InstanceID))
	if err != nil {
		return err
	}
	rt := model.ComputeRuntime{StopTemplate: computeStopDocker}
	if in.ProfileID > 0 {
		if _, r, err := computeLoadSpec(in.ProfileID); err == nil {
			rt = r
		}
	}
	hosts := computeHostMap()
	computeDrain(run, in)
	if in.ContainerName != "" || in.VllmDeploymentID > 0 {
		if err := run.step("stop", func() (string, error) { return computeStopHostsMode(in, rt, spec.HostIDs, hosts, remove) }); err != nil {
			_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstFailed, "last_error": computeTrim(err.Error(), 500)})
			return err
		}
	}
	if remove {
		computeState.dropInst(in.ID)
		return repo.ComputeInstanceRepo.Delete(repo.WithByID(in.ID))
	}
	return repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstStopped})
}

func computeRunStart(run *computeJobRun, spec computeJobSpec) error {
	in, err := repo.ComputeInstanceRepo.Get(repo.WithByID(spec.InstanceID))
	if err != nil {
		return err
	}
	prof, rt, err := computeLoadSpec(in.ProfileID)
	if err != nil {
		return err
	}
	hosts := computeHostMap()
	if spec.Type == "restart" {
		computeDrain(run, in)
	}
	_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstStarting})
	key := computeInstanceKey(in)
	if err := computeLaunch(run, in, prof, rt, spec.HostIDs, hosts, key); err != nil {
		_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstFailed, "last_error": computeTrim(err.Error(), 500)})
		return err
	}
	_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstRunning, "route_enabled": true, "last_error": ""})
	computeRouterInvalidate()
	return nil
}

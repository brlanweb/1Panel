package service

import (
	"context"
	"encoding/json"
	"fmt"
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
}

type computeJobRun struct {
	job   model.ComputeJob
	steps []dto.ComputeJobStep
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

func (r *computeJobRun) save() {
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

func computePrecheck(prof model.ComputeProfile, rt model.ComputeRuntime, hostIDs []uint, excludeInst uint) dto.ComputePrecheckRes {
	res := dto.ComputePrecheckRes{OK: true}
	add := func(name string, ok bool, msg string) {
		res.Items = append(res.Items, dto.ComputePrecheckItem{Name: name, OK: ok, Message: msg})
		if !ok {
			res.OK = false
		}
	}
	hosts := computeHostMap()
	insts, _ := repo.ComputeInstanceRepo.List()
	busy := map[uint]string{}
	portUsed := map[uint]map[int]bool{}
	for _, in := range insts {
		if in.ID == excludeInst || in.Status == model.ComputeInstStopped {
			continue
		}
		for _, id := range computeParseIDs(in.HostIDs) {
			busy[id] = in.Name
			if portUsed[id] == nil {
				portUsed[id] = map[int]bool{}
			}
			portUsed[id][in.Port] = true
		}
	}
	if len(hostIDs) == 0 {
		hostIDs = computePlace(prof, rt, hosts, busy)
		if len(hostIDs) == 0 {
			add("placement", false, fmt.Sprintf("no %d free host(s) satisfy the profile", prof.NodeCount))
			return res
		}
		add("placement", true, strings.Join(computeHostNames(hostIDs, hosts), ", "))
	}
	res.HostIDs = hostIDs
	add("nodeCount", len(hostIDs) == prof.NodeCount, fmt.Sprintf("%d / %d", len(hostIDs), prof.NodeCount))
	if prof.NodeCount > 1 && !rt.MultiNode {
		add("runtime", false, "runtime "+rt.Name+" is single node")
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
		if name, ok := busy[id]; ok {
			add(h.Name+":free", false, "used by "+name)
		}
		if portUsed[id][prof.Port] {
			add(h.Name+":port", false, fmt.Sprintf("port %d in use", prof.Port))
		}
		if slow := computeState.hostSlow(id); slow != nil {
			for _, p := range slow.Procs {
				if p.Port == prof.Port && excludeInst == 0 {
					add(h.Name+":port", false, fmt.Sprintf("port %d used by pid %d", prof.Port, p.Pid))
				}
			}
		}
		if prof.MinMemGB > 0 && rtm.Online {
			need := uint64(prof.MinMemGB) << 30
			// The old instance of a switch frees its memory first.
			avail, pool := computeFreeMem(rtm, rt.Device, excludeInst > 0)
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
	if c, ok := computeModelPathCheck(prof, hostIDs, hosts); !ok {
		add("modelPath", false, c)
	} else {
		add("modelPath", true, c)
	}
	return res
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

func computeModelPathCheck(prof model.ComputeProfile, ids []uint, hosts map[uint]model.ComputeHost) (string, bool) {
	var missing []string
	script := fmt.Sprintf("test -e %s && echo OK || echo MISSING", computeShellQuote(prof.ModelPath))
	for _, id := range ids {
		h, ok := hosts[id]
		if !ok {
			continue
		}
		out, err := computeSSH.RunTimeout(h, script, false, 15*time.Second)
		if err != nil || !strings.Contains(out, "OK") {
			missing = append(missing, h.Name)
		}
	}
	if len(missing) > 0 {
		return "model not found on " + strings.Join(missing, ", "), false
	}
	return prof.ModelPath, true
}

// computePlace picks hosts for a profile: free, online, schedulable, enough
// memory; multi-host profiles take a whole connected fabric group. Prefers
// the least loaded hosts.
func computePlace(prof model.ComputeProfile, runtime model.ComputeRuntime, hosts map[uint]model.ComputeHost, busy map[uint]string) []uint {
	var free []uint
	avail := map[uint]uint64{}
	for id, h := range hosts {
		if !h.Schedulable || busy[id] != "" {
			continue
		}
		rt := computeState.hostRealtime(id)
		if !rt.Online || computeAccelCount(rt, runtime.Device) == 0 {
			continue
		}
		mem, _ := computeFreeMem(rt, runtime.Device, false)
		if prof.MinMemGB > 0 && mem+computeMemHeadroom < uint64(prof.MinMemGB)<<30 {
			continue
		}
		avail[id] = mem
		free = append(free, id)
	}
	sort.Slice(free, func(i, j int) bool {
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
		name = fmt.Sprintf("%s@%s", prof.ServedName, hosts[res.HostIDs[0]].Name)
	}
	if _, err := repo.ComputeInstanceRepo.Get(repo.WithByName(name)); err == nil {
		return res, buserr.New("ErrRecordExist")
	}
	id, err := computeStartJob(computeJobSpec{Type: "deploy", ProfileID: prof.ID, ServedName: prof.ServedName,
		HostIDs: res.HostIDs, Source: "manual", Name: name})
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
	if len(ids) != prof.NodeCount {
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
		ServedName: prof.ServedName, HostIDs: ids, Source: "manual"})
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
	in := model.ComputeInstance{Name: spec.Name, ProfileID: prof.ID, ServedName: prof.ServedName, HeadHostID: head.ID,
		HostIDs: computeJoinIDs(spec.HostIDs), Port: prof.Port, Endpoint: computeEndpoint(head.Addr, prof.Port),
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

// computeLaunch: render → start workers → start head → wait healthy → warm up.
func computeLaunch(run *computeJobRun, in model.ComputeInstance, prof model.ComputeProfile, rt model.ComputeRuntime,
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
			return computeSSH.RunTimeout(h, script, true, computeStepTimeout)
		}); err != nil {
			return err
		}
	}
	timeout := time.Duration(prof.StartTimeoutSec) * time.Second
	if err := run.step("wait healthy", func() (string, error) {
		return computeWaitHealthy(in, rt, key, timeout)
	}); err != nil {
		return err
	}
	_ = run.step("warm up", func() (string, error) { return computeWarmup(in, key) })
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

func computeWarmup(in model.ComputeInstance, key string) (string, error) {
	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"max_tokens":1}`, in.ServedName)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(in.Endpoint, "/")+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	return fmt.Sprintf("http %d", resp.StatusCode), nil
}

func computeStopHosts(in model.ComputeInstance, rt model.ComputeRuntime, ids []uint, hosts map[uint]model.ComputeHost) (string, error) {
	var msgs []string
	var firstErr error
	for i, id := range ids {
		h := hosts[id]
		script, err := computeRender("stop", rt.StopTemplate, computeTplVars{Name: in.Name, ContainerName: in.ContainerName,
			Port: in.Port, Env: map[string]string{}, Rank: i, IsHead: i == 0, NodeCount: len(ids)})
		if err != nil {
			return "", err
		}
		out, err := computeSSH.RunTimeout(h, script, true, 2*time.Minute)
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
	if err := run.step("stop old", func() (string, error) { return computeStopHosts(in, oldRt, ids, hosts) }); err != nil {
		return false, err
	}
	key := computeInstanceKey(in)
	if key == "" {
		key = computeRandomKey("sk-node-")
	}
	keyCipher, _ := computeEncrypt(key)
	next := in
	next.ProfileID, next.ServedName, next.Port = newProf.ID, newProf.ServedName, newProf.Port
	next.Endpoint = computeEndpoint(hosts[ids[0]].Addr, newProf.Port)
	if next.ContainerName == "" {
		next.ContainerName = computeContainerName(in.Name)
	}
	_ = repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"status": model.ComputeInstStarting, "profile_id": next.ProfileID,
		"served_name": next.ServedName, "port": next.Port, "endpoint": next.Endpoint, "container_name": next.ContainerName,
		"api_key_cipher": keyCipher, "adopted": false})
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
			"port": in.Port, "endpoint": in.Endpoint})
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
	if in.ContainerName != "" {
		if err := run.step("stop", func() (string, error) { return computeStopHosts(in, rt, spec.HostIDs, hosts) }); err != nil {
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

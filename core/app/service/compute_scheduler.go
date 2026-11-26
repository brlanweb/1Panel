package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
	"gorm.io/gorm"
)

// Slow scheduler (core side). The fast path — health ejection and
// load-aware weighting — lives in the router.
//
// Every 30s it evaluates each policy against live metrics and produces
// suggestions:
//   minViolation  healthy instances below MinInstances
//   hotSpare      fewer idle-capable spare hosts than HotSpare
//   scaleUp       queue / KV pressure sustained for SustainRounds rounds
//   scaleDown     an instance idle for IdleMinutes above MinInstances
//   preempt       scale-up blocked by capacity while a lower priority,
//                 preemptible model holds hosts
//   fabric        RDMA errors / link down on a domain hosting instances
// In "auto" mode deploy / scaleDown suggestions are executed (one action
// per round, per-model cooldown); preemption is always manual. A global
// pause switch turns automation off without touching policies.

const (
	computeSchedTick   = 30 * time.Second
	computeSuggestKeep = 10 * time.Minute
)

type computeSchedConf struct {
	Paused bool `json:"paused"`
}

func computeLoadSchedConf() computeSchedConf {
	var c computeSchedConf
	if v, err := settingRepo.GetValueByKey(computeSettingSched); err == nil && v != "" {
		_ = json.Unmarshal([]byte(v), &c)
	}
	return c
}

type computeSchedState struct {
	mu          sync.Mutex
	suggestions map[string]dto.ComputeSuggestion
	dismissed   map[string]time.Time
	lastAction  map[string]time.Time // servedName -> last automatic / applied action
}

var computeSched = &computeSchedState{
	suggestions: map[string]dto.ComputeSuggestion{},
	dismissed:   map[string]time.Time{},
	lastAction:  map[string]time.Time{},
}

func computeExpr(expr string, args ...interface{}) interface{} { return gorm.Expr(expr, args...) }

func computeSchedulerLoop() {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("compute scheduler panic: %v", r)
			time.Sleep(computeSchedTick)
			go computeSchedulerLoop()
		}
	}()
	go computeUsageFlushLoop()
	go computeCallLogJanitor()
	time.Sleep(45 * time.Second)
	ticker := time.NewTicker(computeSchedTick)
	defer ticker.Stop()
	for {
		computeSchedRound()
		<-ticker.C
	}
}

type computeModelState struct {
	policy  model.ComputePolicy
	insts   []model.ComputeInstance
	healthy []model.ComputeInstance
	waiting float64
	kvMax   float64
	running float64
	// ttft is the request-weighted mean TTFT (ms) of the healthy instances.
	ttft  float64
	ttftW float64
}

// computePressured reports whether a model is above its scale-up
// thresholds: average queue per instance, peak KV cache usage or (when
// set) the mean time to first token.
func computePressured(p model.ComputePolicy, avgWait, kvMax, ttftMs float64) bool {
	if avgWait >= p.QueueHigh || kvMax >= p.KVHigh {
		return true
	}
	return p.TTFTHigh > 0 && ttftMs >= p.TTFTHigh
}

func computeSchedRound() {
	policies, err := repo.ComputePolicyRepo.List()
	if err != nil {
		return
	}
	insts, _ := repo.ComputeInstanceRepo.List()
	hosts := computeHostMap()
	now := time.Now()
	fresh := map[string]dto.ComputeSuggestion{}
	add := func(s dto.ComputeSuggestion) {
		s.CreatedAt = now.UnixMilli()
		fresh[s.ID] = s
	}

	busy := map[uint]string{}
	for _, in := range insts {
		if in.Status == model.ComputeInstStopped {
			continue
		}
		for _, id := range computeParseIDs(in.HostIDs) {
			busy[id] = in.ServedName
		}
	}
	freeHosts := 0
	for id, h := range hosts {
		rt := computeState.hostRealtime(id)
		if h.Schedulable && rt.Online && busy[id] == "" && len(rt.GPUs) > 0 {
			freeHosts++
		}
	}

	states := map[string]*computeModelState{}
	for _, p := range policies {
		states[p.ServedName] = &computeModelState{policy: p}
	}
	for _, in := range insts {
		st := states[in.ServedName]
		if st == nil || in.Status == model.ComputeInstStopped {
			continue
		}
		st.insts = append(st.insts, in)
		m := computeState.instMetrics(in.ID)
		if m.Healthy && in.Status == model.ComputeInstRunning {
			st.healthy = append(st.healthy, in)
			st.waiting += m.Waiting
			st.running += m.Running
			if m.KVUsage > st.kvMax {
				st.kvMax = m.KVUsage
			}
			if m.TTFTMs > 0 {
				w := m.Running + m.Waiting + 1
				st.ttft += m.TTFTMs * w
				st.ttftW += w
			}
		}
	}
	for _, st := range states {
		if st.ttftW > 0 {
			st.ttft /= st.ttftW
		}
	}

	// Pressure counters live on the per-instance state of the first
	// healthy instance of each model (cheap and survives policy edits).
	for name, st := range states {
		p := st.policy
		if p.Mode == "off" {
			continue
		}
		prof, profErr := repo.ComputeProfileRepo.Get(repo.WithByID(p.ProfileID))
		hasProfile := profErr == nil && p.ProfileID > 0
		need := 1
		// canPlace: one more replica fits (free hosts, or room on a host
		// that stacks several single-host replicas).
		canPlace := false
		if hasProfile {
			need = max(prof.NodeCount, 1)
			canPlace = computeCanPlace(prof)
		}
		n := len(st.healthy)

		if n < p.MinInstances && len(st.insts) < max(p.MaxInstances, p.MinInstances) {
			add(dto.ComputeSuggestion{ID: "min:" + name, Kind: "minViolation", ServedName: name, ProfileID: p.ProfileID,
				Severity: "critical", Actionable: canPlace,
				Reason: fmt.Sprintf("healthy %d < min %d", n, p.MinInstances)})
		}

		pressured := false
		if n > 0 {
			avgWait := st.waiting / float64(n)
			pressured = computePressured(p, avgWait, st.kvMax, st.ttft)
			lead := computeState.inst(st.healthy[0].ID)
			computeState.mu.Lock()
			if pressured {
				lead.pressure++
			} else {
				lead.pressure = 0
			}
			rounds := lead.pressure
			computeState.mu.Unlock()
			if pressured && rounds >= max(p.SustainRounds, 1) && len(st.insts) < p.MaxInstances {
				reason := fmt.Sprintf("avg waiting %.1f (≥%.1f) / max KV %.0f%% (≥%.0f%%)", avgWait, p.QueueHigh, st.kvMax*100, p.KVHigh*100)
				if p.TTFTHigh > 0 {
					reason += fmt.Sprintf(" / TTFT %.0f ms (≥%.0f ms)", st.ttft, p.TTFTHigh)
				}
				reason += fmt.Sprintf(" for %d rounds", rounds)
				if canPlace {
					add(dto.ComputeSuggestion{ID: "up:" + name, Kind: "scaleUp", ServedName: name, ProfileID: p.ProfileID,
						Severity: "warning", Actionable: true, Reason: reason})
				} else if victim, ok := computePreemptVictim(p, states, need); ok {
					add(dto.ComputeSuggestion{ID: "preempt:" + name, Kind: "preempt", ServedName: name, ProfileID: p.ProfileID,
						InstanceID: victim.ID, HostIDs: computeParseIDs(victim.HostIDs), Severity: "warning", Actionable: true,
						Reason: reason + "; no free host, can preempt " + victim.Name + " (" + victim.ServedName + ")"})
				} else {
					add(dto.ComputeSuggestion{ID: "cap:" + name, Kind: "scaleUp", ServedName: name, ProfileID: p.ProfileID,
						Severity: "warning", Actionable: false, Reason: reason + "; no capacity"})
				}
			}
		}

		if !pressured && n > max(p.MinInstances, 0) && p.IdleMinutes > 0 {
			// Pick the instance idle the longest.
			var cand *model.ComputeInstance
			var since time.Time
			for i := range st.healthy {
				in := st.healthy[i]
				if in.Adopted {
					continue
				}
				is := computeState.inst(in.ID)
				computeState.mu.Lock()
				idle := is.idleSince
				computeState.mu.Unlock()
				if idle.IsZero() || now.Sub(idle) < time.Duration(p.IdleMinutes)*time.Minute {
					continue
				}
				if cand == nil || idle.Before(since) {
					cand, since = &st.healthy[i], idle
				}
			}
			if cand != nil {
				add(dto.ComputeSuggestion{ID: fmt.Sprintf("down:%s:%d", name, cand.ID), Kind: "scaleDown", ServedName: name,
					InstanceID: cand.ID, HostIDs: computeParseIDs(cand.HostIDs), Severity: "info", Actionable: true,
					Reason: fmt.Sprintf("%s idle for %s", cand.Name, now.Sub(since).Round(time.Minute))})
			}
		}

		if p.HotSpare > 0 && freeHosts < p.HotSpare*need {
			add(dto.ComputeSuggestion{ID: "spare:" + name, Kind: "hotSpare", ServedName: name, Severity: "info",
				Reason: fmt.Sprintf("free hosts %d < hot spare %d×%d", freeHosts, p.HotSpare, need)})
		}
	}

	// Fabric health of domains that carry multi-host instances.
	for _, in := range insts {
		ids := computeParseIDs(in.HostIDs)
		if len(ids) < 2 || in.Status == model.ComputeInstStopped {
			continue
		}
		var bad []string
		for _, id := range ids {
			for _, r := range computeState.hostRealtime(id).Rdma {
				if r.ErrDelta > 0 || (r.NetDev != "" && r.State != "ACTIVE" && computeLinkUses(id, r.NetDev)) {
					bad = append(bad, fmt.Sprintf("%s/%s", hosts[id].Name, r.Dev))
				}
			}
		}
		if len(bad) > 0 {
			add(dto.ComputeSuggestion{ID: fmt.Sprintf("fabric:%d", in.ID), Kind: "fabric", ServedName: in.ServedName,
				InstanceID: in.ID, HostIDs: ids, Severity: "critical", Reason: "RDMA errors / link down: " + strings.Join(bad, ", ")})
		}
	}

	computeSched.mu.Lock()
	for id, t := range computeSched.dismissed {
		if now.Sub(t) > computeSuggestKeep {
			delete(computeSched.dismissed, id)
		}
	}
	for id := range fresh {
		if _, ok := computeSched.dismissed[id]; ok {
			delete(fresh, id)
		} else if old, ok := computeSched.suggestions[id]; ok {
			s := fresh[id]
			s.CreatedAt = old.CreatedAt
			fresh[id] = s
		}
	}
	computeSched.suggestions = fresh
	computeSched.mu.Unlock()

	if computeLoadSchedConf().Paused {
		return
	}
	computeAutoAct(fresh, states, now)
}

func computeLinkUses(hostID uint, iface string) bool {
	links, _ := computeLinks()
	for _, l := range links {
		if (l.HostAID == hostID && l.IfaceA == iface) || (l.HostBID == hostID && l.IfaceB == iface) {
			return true
		}
	}
	return false
}

// computePreemptVictim finds an instance of a lower priority, preemptible
// model with enough hosts, never taking a model below its minimum.
func computePreemptVictim(p model.ComputePolicy, states map[string]*computeModelState, need int) (model.ComputeInstance, bool) {
	var cands []model.ComputeInstance
	for name, st := range states {
		if name == p.ServedName || !st.policy.Preemptible || st.policy.Priority >= p.Priority {
			continue
		}
		if len(st.healthy) <= st.policy.MinInstances {
			continue
		}
		for _, in := range st.insts {
			if !in.Adopted && len(computeParseIDs(in.HostIDs)) >= need {
				cands = append(cands, in)
			}
		}
	}
	if len(cands) == 0 {
		return model.ComputeInstance{}, false
	}
	sort.Slice(cands, func(i, j int) bool {
		return states[cands[i].ServedName].policy.Priority < states[cands[j].ServedName].policy.Priority
	})
	return cands[0], true
}

func computeAutoAct(sugs map[string]dto.ComputeSuggestion, states map[string]*computeModelState, now time.Time) {
	// One automatic action per round; deploys before scale-downs.
	ordered := make([]dto.ComputeSuggestion, 0, len(sugs))
	for _, s := range sugs {
		ordered = append(ordered, s)
	}
	rank := map[string]int{"minViolation": 0, "scaleUp": 1, "scaleDown": 2}
	sort.Slice(ordered, func(i, j int) bool { return rank[ordered[i].Kind] < rank[ordered[j].Kind] })
	for _, s := range ordered {
		st := states[s.ServedName]
		if st == nil || st.policy.Mode != "auto" || !s.Actionable {
			continue
		}
		if _, ok := rank[s.Kind]; !ok {
			continue
		}
		computeSched.mu.Lock()
		last := computeSched.lastAction[s.ServedName]
		computeSched.mu.Unlock()
		if now.Sub(last) < time.Duration(max(st.policy.CooldownSec, 60))*time.Second {
			continue
		}
		if _, err := computeApplySuggestion(s, "auto"); err != nil {
			global.LOG.Warnf("compute auto %s %s: %v", s.Kind, s.ServedName, err)
			continue
		}
		return
	}
}

func computeApplySuggestion(s dto.ComputeSuggestion, source string) (uint, error) {
	var id uint
	var err error
	switch s.Kind {
	case "minViolation", "scaleUp":
		prof, perr := repo.ComputeProfileRepo.Get(repo.WithByID(s.ProfileID))
		if perr != nil {
			return 0, buserr.New("ErrRecordNotFound")
		}
		rt, rerr := repo.ComputeRuntimeRepo.Get(repo.WithByID(prof.RuntimeID))
		if rerr != nil {
			return 0, buserr.New("ErrRecordNotFound")
		}
		pre := computePrecheck(prof, rt, nil, 0)
		if !pre.OK {
			return 0, buserr.WithDetail("ErrComputePrecheck", computePrecheckText(pre), nil)
		}
		hosts := computeHostMap()
		name := computeReplicaName(prof.ServedName, hosts[pre.HostIDs[0]].Name)
		id, err = computeStartJob(computeJobSpec{Type: "deploy", ProfileID: prof.ID, ServedName: prof.ServedName,
			HostIDs: pre.HostIDs, Source: source, Reason: s.Reason, Name: name, Port: pre.Port})
	case "scaleDown":
		in, ierr := repo.ComputeInstanceRepo.Get(repo.WithByID(s.InstanceID))
		if ierr != nil {
			return 0, buserr.New("ErrRecordNotFound")
		}
		id, err = computeStartJob(computeJobSpec{Type: "scaleDown", InstanceID: in.ID, ProfileID: in.ProfileID,
			ServedName: in.ServedName, HostIDs: computeParseIDs(in.HostIDs), Source: source, Reason: s.Reason})
	case "preempt":
		// Switch the victim instance to the starved model's profile: the
		// hosts are reused, the victim drains first.
		in, ierr := repo.ComputeInstanceRepo.Get(repo.WithByID(s.InstanceID))
		if ierr != nil {
			return 0, buserr.New("ErrRecordNotFound")
		}
		id, err = computeStartJob(computeJobSpec{Type: "switch", InstanceID: in.ID, ProfileID: s.ProfileID,
			ServedName: s.ServedName, HostIDs: computeParseIDs(in.HostIDs), Source: source, Reason: s.Reason})
	default:
		return 0, buserr.WithDetail("ErrInvalidParams", s.Kind, nil)
	}
	if err == nil {
		computeSched.mu.Lock()
		computeSched.lastAction[s.ServedName] = time.Now()
		delete(computeSched.suggestions, s.ID)
		computeSched.mu.Unlock()
	}
	return id, err
}

func computePrecheckText(p dto.ComputePrecheckRes) string {
	var fails []string
	for _, it := range p.Items {
		if !it.OK {
			fails = append(fails, it.Name+": "+it.Message)
		}
	}
	return strings.Join(fails, "; ")
}

// ---- service methods ------------------------------------------------------

func (u *ComputeService) ListPolicies() ([]model.ComputePolicy, error) {
	return repo.ComputePolicyRepo.List(repo.WithComputeOrder("priority desc, served_name asc"))
}

func (u *ComputeService) SavePolicy(req dto.ComputePolicyReq) error {
	_, err := computeSavePolicy(req, nil)
	return err
}

// computeSavePolicy validates and stores a policy; extra columns (pool
// fields) are only applied on create.
func computeSavePolicy(req dto.ComputePolicyReq, extra *model.ComputePolicy) (uint, error) {
	if req.MaxInstances < req.MinInstances {
		return 0, buserr.WithDetail("ErrInvalidParams", "maxInstances < minInstances", nil)
	}
	if req.Mode == "auto" && req.ProfileID == 0 {
		return 0, buserr.WithDetail("ErrInvalidParams", "auto mode requires a profile", nil)
	}
	if req.QueueHigh <= 0 {
		req.QueueHigh = 4
	}
	if req.KVHigh <= 0 || req.KVHigh > 1 {
		req.KVHigh = 0.92
	}
	if req.TTFTHigh < 0 {
		req.TTFTHigh = 0
	}
	if req.SustainRounds <= 0 {
		req.SustainRounds = 3
	}
	if req.CooldownSec < 60 {
		req.CooldownSec = 600
	}
	if dup, err := repo.ComputePolicyRepo.Get(repo.WithComputeField("served_name", req.ServedName)); err == nil && dup.ID != req.ID {
		return 0, buserr.New("ErrRecordExist")
	}
	vars := map[string]interface{}{"served_name": req.ServedName, "profile_id": req.ProfileID, "mode": req.Mode,
		"min_instances": req.MinInstances, "max_instances": req.MaxInstances, "hot_spare": req.HotSpare, "priority": req.Priority,
		"preemptible": req.Preemptible, "queue_high": req.QueueHigh, "kv_high": req.KVHigh, "ttft_high": req.TTFTHigh,
		"sustain_rounds": req.SustainRounds, "idle_minutes": req.IdleMinutes, "cooldown_sec": req.CooldownSec}
	if req.ID > 0 {
		return req.ID, repo.ComputePolicyRepo.Update(req.ID, vars)
	}
	p := model.ComputePolicy{ServedName: req.ServedName, ProfileID: req.ProfileID, Mode: req.Mode, MinInstances: req.MinInstances,
		MaxInstances: req.MaxInstances, HotSpare: req.HotSpare, Priority: req.Priority, Preemptible: req.Preemptible,
		QueueHigh: req.QueueHigh, KVHigh: req.KVHigh, TTFTHigh: req.TTFTHigh, SustainRounds: req.SustainRounds,
		IdleMinutes: req.IdleMinutes, CooldownSec: req.CooldownSec}
	if extra != nil {
		p.Name, p.AccountID, p.ConsumerID = extra.Name, extra.AccountID, extra.ConsumerID
	}
	err := repo.ComputePolicyRepo.Create(&p)
	return p.ID, err
}

func (u *ComputeService) DeletePolicy(id uint) error {
	return repo.ComputePolicyRepo.Delete(repo.WithByID(id))
}

func (u *ComputeService) Suggestions() []dto.ComputeSuggestion {
	computeSched.mu.Lock()
	defer computeSched.mu.Unlock()
	res := make([]dto.ComputeSuggestion, 0, len(computeSched.suggestions))
	for _, s := range computeSched.suggestions {
		res = append(res, s)
	}
	sev := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.Slice(res, func(i, j int) bool {
		if sev[res[i].Severity] != sev[res[j].Severity] {
			return sev[res[i].Severity] < sev[res[j].Severity]
		}
		return res[i].ID < res[j].ID
	})
	return res
}

func (u *ComputeService) OperateSuggestion(req dto.ComputeSuggestionOpReq) (uint, error) {
	computeSched.mu.Lock()
	s, ok := computeSched.suggestions[req.ID]
	if ok && req.Operation == "dismiss" {
		delete(computeSched.suggestions, req.ID)
		computeSched.dismissed[req.ID] = time.Now()
	}
	computeSched.mu.Unlock()
	if !ok {
		return 0, buserr.New("ErrRecordNotFound")
	}
	if req.Operation == "dismiss" {
		return 0, nil
	}
	if !s.Actionable {
		return 0, buserr.WithDetail("ErrInvalidParams", "not actionable", nil)
	}
	return computeApplySuggestion(s, "suggestion")
}

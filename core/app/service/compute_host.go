package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
)

type ComputeService struct{}

type IComputeService interface {
	// hosts
	ListHosts(info string) ([]dto.ComputeHostView, error)
	HostCandidates(source string) ([]dto.ComputeHostCandidate, error)
	ImportHosts(req dto.ComputeHostImportReq) (dto.ComputeHostImportRes, error)
	UpdateHost(req dto.ComputeHostReq) error
	DeleteHost(id uint) error
	SetSchedulable(req dto.ComputeSchedulableReq) error
	RefreshHost(id uint) error
	Watch(fast bool)

	// topology
	Topology() ([]dto.ComputeFabricDomain, error)
	CreateLink(req dto.ComputeLinkReq) error
	DeleteLink(id uint) error

	// templates
	ListRuntimes() ([]model.ComputeRuntime, error)
	SaveRuntime(req dto.ComputeRuntimeReq) error
	DeleteRuntime(id uint) error
	ListProfiles() ([]model.ComputeProfile, error)
	SaveProfile(req dto.ComputeProfileReq) error
	DeleteProfile(id uint) error

	// instances
	ListInstances() ([]dto.ComputeInstanceView, error)
	SaveInstance(req dto.ComputeInstanceReq) error
	Adopt(req dto.ComputeAdoptReq) error
	SetRoute(req dto.ComputeRouteReq) error
	OperateInstance(req dto.ComputeInstanceOpReq) (uint, error)

	// jobs
	Deploy(req dto.ComputeDeployReq) (dto.ComputePrecheckRes, error)
	Switch(req dto.ComputeSwitchReq) (dto.ComputePrecheckRes, error)
	PageJobs(req dto.SearchWithPage) (int64, []model.ComputeJob, error)
	GetJob(id uint) (model.ComputeJob, error)

	// router
	ListConsumers() ([]model.ComputeConsumer, error)
	SaveConsumer(req dto.ComputeConsumerReq) (dto.ComputeConsumerKeyRes, error)
	ResetConsumerKey(id uint) (dto.ComputeConsumerKeyRes, error)
	DeleteConsumer(id uint) error
	GetRouterSetting() (dto.ComputeRouterSetting, error)
	UpdateRouterSetting(req dto.ComputeRouterSetting) error

	// scheduler
	ListPolicies() ([]model.ComputePolicy, error)
	SavePolicy(req dto.ComputePolicyReq) error
	DeletePolicy(id uint) error
	Suggestions() []dto.ComputeSuggestion
	OperateSuggestion(req dto.ComputeSuggestionOpReq) (uint, error)

	Overview() (dto.ComputeOverview, error)
}

func NewIComputeService() IComputeService { return &ComputeService{} }

// ---- hosts ---------------------------------------------------------------

func (u *ComputeService) ListHosts(info string) ([]dto.ComputeHostView, error) {
	hosts, err := repo.ComputeHostRepo.List(repo.WithComputeLike([]string{"name", "addr", "labels"}, info), repo.WithComputeOrder("name asc"))
	if err != nil {
		return nil, err
	}
	links, _ := repo.ComputeLinkRepo.List()
	domainOf := map[uint]string{}
	for _, l := range links {
		domainOf[l.HostAID] = l.Domain
		domainOf[l.HostBID] = l.Domain
	}
	insts, _ := repo.ComputeInstanceRepo.List()
	instOf := map[uint][]string{}
	for _, in := range insts {
		for _, hid := range computeParseIDs(in.HostIDs) {
			instOf[hid] = append(instOf[hid], in.Name)
		}
	}
	terms := map[uint]computeTerminalHost{}
	if list, err := computeTerminalHosts(); err == nil {
		for _, t := range list {
			terms[t.ID] = t
		}
	}
	nodes := map[uint]model.ClusterNode{}
	if list, err := clusterNodeRepo.GetList(); err == nil {
		for _, n := range list {
			nodes[n.ID] = n
		}
	}
	res := make([]dto.ComputeHostView, 0, len(hosts))
	for _, h := range hosts {
		rt := computeState.hostRealtime(h.ID)
		if slow := computeState.hostSlow(h.ID); slow != nil {
			rt.Processes = computeProcessesView(h.ID, slow, insts)
		}
		view := dto.ComputeHostView{ComputeHost: h, Realtime: rt, Domain: domainOf[h.ID], InstanceNames: instOf[h.ID]}
		switch h.Source {
		case model.ComputeSourceTerminal:
			if t, ok := terms[h.RefID]; ok {
				view.SourceName = t.Name
				view.SourceDetail = fmt.Sprintf("%s@%s:%d", t.User, t.Addr, t.Port)
			} else {
				view.SourceMissing = true
			}
		case model.ComputeSourceNode:
			if n, ok := nodes[h.RefID]; ok {
				view.SourceName = n.Name
				view.SourceDetail = n.EntryURL
			} else {
				view.SourceMissing = true
			}
		case model.ComputeSourceLocal:
			view.SourceName = "local"
		}
		res = append(res, view)
	}
	return res, nil
}

func computeProcessesView(hostID uint, slow *computeSlowRaw, insts []model.ComputeInstance) []dto.ComputeProcess {
	res := make([]dto.ComputeProcess, 0, len(slow.Procs))
	for _, p := range slow.Procs {
		item := dto.ComputeProcess{Pid: p.Pid, Engine: p.Engine, Port: p.Port, Model: p.Model, ServedName: p.ServedName, Cmd: p.Cmd}
		for _, in := range insts {
			if in.HeadHostID == hostID && in.Port == p.Port {
				item.InstanceID = in.ID
			}
		}
		res = append(res, item)
	}
	return res
}

// HostCandidates lists the hosts of a source that can be brought under
// compute scheduling, marking the ones already imported.
func (u *ComputeService) HostCandidates(source string) ([]dto.ComputeHostCandidate, error) {
	existing, _ := repo.ComputeHostRepo.List()
	imported := map[string]uint{}
	for _, h := range existing {
		imported[fmt.Sprintf("%s/%d", h.Source, h.RefID)] = h.ID
	}
	mark := func(c dto.ComputeHostCandidate) dto.ComputeHostCandidate {
		if id, ok := imported[fmt.Sprintf("%s/%d", c.Source, c.RefID)]; ok {
			c.Imported, c.HostID = true, id
		}
		return c
	}
	var res []dto.ComputeHostCandidate
	switch source {
	case model.ComputeSourceTerminal:
		list, err := computeTerminalHosts()
		if err != nil {
			return nil, err
		}
		for _, t := range list {
			res = append(res, mark(dto.ComputeHostCandidate{Source: source, RefID: t.ID, Name: t.Name, Addr: t.Addr,
				Detail: fmt.Sprintf("%s@%s:%d · %s", t.User, t.Addr, t.Port, t.AuthMode)}))
		}
	case model.ComputeSourceNode:
		list, err := clusterNodeRepo.GetList()
		if err != nil {
			return nil, err
		}
		for _, n := range list {
			res = append(res, mark(dto.ComputeHostCandidate{Source: source, RefID: n.ID, Name: n.Name, Addr: entryHost(n.EntryURL),
				Detail: n.EntryURL, Status: n.Status}))
		}
	case model.ComputeSourceLocal:
		ip, _ := settingRepo.GetValueByKey("SystemIP")
		res = append(res, mark(dto.ComputeHostCandidate{Source: source, Name: "local", Addr: strings.TrimSpace(ip), Status: model.ClusterNodeStatusOnline}))
	default:
		return nil, buserr.WithDetail("ErrInvalidParams", "source", nil)
	}
	return res, nil
}

func (u *ComputeService) ImportHosts(req dto.ComputeHostImportReq) (dto.ComputeHostImportRes, error) {
	var res dto.ComputeHostImportRes
	cands, err := u.HostCandidates(req.Source)
	if err != nil {
		return res, err
	}
	want := map[uint]bool{}
	for _, id := range req.RefIDs {
		want[id] = true
	}
	var errs []string
	for _, c := range cands {
		if req.Source != model.ComputeSourceLocal && !want[c.RefID] {
			continue
		}
		if c.Imported {
			continue
		}
		mode := model.ComputeModeAgentless
		if req.Source != model.ComputeSourceTerminal {
			mode = model.ComputeModeAgent
		}
		name := computeUniqueHostName(c.Name)
		h := model.ComputeHost{Name: name, Addr: c.Addr, Source: req.Source, RefID: c.RefID, Mode: mode,
			Labels: strings.TrimSpace(req.Labels), Schedulable: true, Status: model.ComputeHostUnknown}
		if err := repo.ComputeHostRepo.Create(&h); err != nil {
			errs = append(errs, c.Name+": "+err.Error())
			continue
		}
		res.Imported++
		computeKick(h.ID)
	}
	if len(errs) > 0 {
		res.Error = strings.Join(errs, "; ")
	}
	return res, nil
}

func computeUniqueHostName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "host"
	}
	candidate := name
	for i := 2; ; i++ {
		if _, err := repo.ComputeHostRepo.Get(repo.WithByName(candidate)); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", name, i)
	}
}

// UpdateHost edits scheduling attributes only; access is edited where the
// host is managed (Terminal → Hosts / Cluster → Nodes).
func (u *ComputeService) UpdateHost(req dto.ComputeHostReq) error {
	old, err := repo.ComputeHostRepo.Get(repo.WithByID(req.ID))
	if err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	name := strings.TrimSpace(req.Name)
	if dup, err := repo.ComputeHostRepo.Get(repo.WithByName(name)); err == nil && dup.ID != req.ID {
		return buserr.New("ErrRecordExist")
	}
	vars := map[string]interface{}{"name": name, "addr": strings.TrimSpace(req.Addr),
		"labels": strings.TrimSpace(req.Labels), "remark": req.Remark}
	if req.Schedulable != nil {
		vars["schedulable"] = *req.Schedulable
	}
	if err := repo.ComputeHostRepo.Update(old.ID, vars); err != nil {
		return err
	}
	computeKick(old.ID)
	return nil
}

func (u *ComputeService) DeleteHost(id uint) error {
	insts, _ := repo.ComputeInstanceRepo.List()
	for _, in := range insts {
		for _, hid := range computeParseIDs(in.HostIDs) {
			if hid == id {
				return buserr.WithDetail("ErrComputeHostInUse", in.Name, nil)
			}
		}
	}
	if err := repo.ComputeHostRepo.Delete(repo.WithByID(id)); err != nil {
		return err
	}
	_ = computeDBDelete(&model.ComputeFabricLink{}, "host_a_id = ? or host_b_id = ?", id, id)
	computeSSH.Drop(id)
	computeState.dropHost(id)
	return nil
}

func (u *ComputeService) SetSchedulable(req dto.ComputeSchedulableReq) error {
	return repo.ComputeHostRepo.Update(req.ID, map[string]interface{}{"schedulable": req.Schedulable})
}

func (u *ComputeService) RefreshHost(id uint) error {
	if _, err := repo.ComputeHostRepo.Get(repo.WithByID(id)); err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	computeSSH.Drop(id)
	computeKick(id)
	return nil
}

func (u *ComputeService) Watch(fast bool) {
	if fast {
		computeState.watchUntil.Store(time.Now().Add(30 * time.Second).Unix())
	} else {
		computeState.watchUntil.Store(0)
	}
}

// ---- helpers -------------------------------------------------------------

func computeParseIDs(s string) []uint {
	var res []uint
	for _, p := range strings.Split(s, ",") {
		if v, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64); err == nil && v > 0 {
			res = append(res, uint(v))
		}
	}
	return res
}

func computeJoinIDs(ids []uint) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(parts, ",")
}

func computeHostMap() map[uint]model.ComputeHost {
	hosts, _ := repo.ComputeHostRepo.List()
	m := make(map[uint]model.ComputeHost, len(hosts))
	for _, h := range hosts {
		m[h.ID] = h
	}
	return m
}

func computeHostNames(ids []uint, hosts map[uint]model.ComputeHost) []string {
	res := make([]string, 0, len(ids))
	for _, id := range ids {
		if h, ok := hosts[id]; ok {
			res = append(res, h.Name)
		} else {
			res = append(res, fmt.Sprintf("#%d", id))
		}
	}
	return res
}

func computeSortedIDs(m map[uint]bool) []uint {
	res := make([]uint, 0, len(m))
	for id := range m {
		res = append(res, id)
	}
	sort.Slice(res, func(i, j int) bool { return res[i] < res[j] })
	return res
}

func computeDBDelete(value interface{}, query string, args ...interface{}) error {
	return global.DB.Where(query, args...).Delete(value).Error
}

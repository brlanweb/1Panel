package service

import (
	"context"
	"errors"
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
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
)

type ComputeService struct{}

type IComputeService interface {
	// hosts
	ListHosts(info string) ([]dto.ComputeHostView, error)
	CreateHost(req dto.ComputeHostReq) error
	BatchCreateHosts(req dto.ComputeHostBatchReq) (int, error)
	UpdateHost(req dto.ComputeHostReq) error
	DeleteHost(id uint) error
	TestHost(req dto.ComputeHostReq) (dto.ComputeHostTestRes, error)
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
	res := make([]dto.ComputeHostView, 0, len(hosts))
	for _, h := range hosts {
		rt := computeState.hostRealtime(h.ID)
		if slow := computeState.hostSlow(h.ID); slow != nil {
			rt.Processes = computeProcessesView(h.ID, slow, insts)
		}
		res = append(res, dto.ComputeHostView{
			ComputeHost:   h,
			HasCredential: h.CredCipher != "",
			Realtime:      rt,
			Domain:        domainOf[h.ID],
			InstanceNames: instOf[h.ID],
		})
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

func computeHostFromReq(req dto.ComputeHostReq, old *model.ComputeHost) (model.ComputeHost, error) {
	var h model.ComputeHost
	if old != nil {
		h = *old
	} else {
		h.Mode = model.ComputeModeAgentless
		h.Status = model.ComputeHostUnknown
		h.Schedulable = true
	}
	h.Name = strings.TrimSpace(req.Name)
	h.Addr = strings.TrimSpace(req.Addr)
	h.SSHPort = req.SSHPort
	if h.SSHPort == 0 {
		h.SSHPort = 22
	}
	h.SSHUser = strings.TrimSpace(req.SSHUser)
	if h.SSHUser == "" {
		h.SSHUser = "root"
	}
	h.AuthMode = req.AuthMode
	if h.AuthMode == "" {
		h.AuthMode = "password"
	}
	h.Labels = strings.TrimSpace(req.Labels)
	h.Remark = req.Remark
	h.ClusterNodeID = req.ClusterNodeID
	if req.ClusterNodeID > 0 {
		h.Mode = model.ComputeModeAgent
	} else {
		h.Mode = model.ComputeModeAgentless
	}
	if req.Schedulable != nil {
		h.Schedulable = *req.Schedulable
	}
	if req.Credential != "" {
		c, err := encrypt.StringEncrypt(req.Credential)
		if err != nil {
			return h, err
		}
		h.CredCipher = c
		h.PassPhraseCipher = ""
		if req.AuthMode == "key" && req.PassPhrase != "" {
			p, err := encrypt.StringEncrypt(req.PassPhrase)
			if err != nil {
				return h, err
			}
			h.PassPhraseCipher = p
		}
	}
	if h.CredCipher == "" {
		return h, buserr.WithDetail("ErrInvalidParams", "credential", nil)
	}
	return h, nil
}

func (u *ComputeService) CreateHost(req dto.ComputeHostReq) error {
	if _, err := repo.ComputeHostRepo.Get(repo.WithByName(strings.TrimSpace(req.Name))); err == nil {
		return buserr.New("ErrRecordExist")
	}
	h, err := computeHostFromReq(req, nil)
	if err != nil {
		return err
	}
	if err := repo.ComputeHostRepo.Create(&h); err != nil {
		return err
	}
	computeKick(h.ID)
	return nil
}

func (u *ComputeService) BatchCreateHosts(req dto.ComputeHostBatchReq) (int, error) {
	created := 0
	var errs []string
	for _, raw := range strings.Split(req.Lines, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, ",")
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		if len(f) < 2 || f[0] == "" || f[1] == "" {
			errs = append(errs, line+": format")
			continue
		}
		item := dto.ComputeHostReq{Name: f[0], Addr: f[1], SSHPort: req.SSHPort, SSHUser: req.SSHUser,
			AuthMode: req.AuthMode, Credential: req.Credential, PassPhrase: req.PassPhrase, Labels: req.Labels}
		if len(f) > 2 && f[2] != "" {
			p, err := strconv.Atoi(f[2])
			if err != nil {
				errs = append(errs, line+": port")
				continue
			}
			item.SSHPort = p
		}
		if len(f) > 3 && f[3] != "" {
			item.SSHUser = f[3]
		}
		if err := u.CreateHost(item); err != nil {
			errs = append(errs, f[0]+": "+err.Error())
			continue
		}
		created++
	}
	if len(errs) > 0 {
		return created, buserr.WithDetail("ErrInvalidParams", strings.Join(errs, "; "), nil)
	}
	return created, nil
}

func (u *ComputeService) UpdateHost(req dto.ComputeHostReq) error {
	old, err := repo.ComputeHostRepo.Get(repo.WithByID(req.ID))
	if err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	if dup, err := repo.ComputeHostRepo.Get(repo.WithByName(strings.TrimSpace(req.Name))); err == nil && dup.ID != req.ID {
		return buserr.New("ErrRecordExist")
	}
	h, err := computeHostFromReq(req, &old)
	if err != nil {
		return err
	}
	if err := repo.ComputeHostRepo.Update(h.ID, map[string]interface{}{
		"name": h.Name, "addr": h.Addr, "ssh_port": h.SSHPort, "ssh_user": h.SSHUser, "auth_mode": h.AuthMode,
		"cred_cipher": h.CredCipher, "pass_phrase_cipher": h.PassPhraseCipher, "mode": h.Mode,
		"cluster_node_id": h.ClusterNodeID, "labels": h.Labels, "remark": h.Remark, "schedulable": h.Schedulable,
	}); err != nil {
		return err
	}
	computeSSH.Drop(h.ID)
	computeKick(h.ID)
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

// TestHost dials the host with the submitted (or stored) credentials using a
// throw-away connection and reads its identity. Nothing is persisted.
func (u *ComputeService) TestHost(req dto.ComputeHostReq) (dto.ComputeHostTestRes, error) {
	var res dto.ComputeHostTestRes
	var old *model.ComputeHost
	if req.ID > 0 {
		h, err := repo.ComputeHostRepo.Get(repo.WithByID(req.ID))
		if err == nil {
			old = &h
		}
	}
	h, err := computeHostFromReq(req, old)
	if err != nil {
		return res, err
	}
	info, err := computeConnInfo(h)
	if err != nil {
		return res, err
	}
	start := time.Now()
	client, err := ssh.NewClient(info)
	if err != nil {
		res.Message = err.Error()
		return res, nil
	}
	defer client.Close()
	res.LatencyMs = time.Since(start).Milliseconds()
	res.Sudo = h.SSHUser == "root" || client.SudoItem != ""
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := computeRunOnClient(ctx, client, computeSlowScript+"\n"+computeFastScript)
	if err != nil && out == "" {
		res.Message = err.Error()
		return res, nil
	}
	slow := parseComputeSlow(out)
	fast := parseComputeFast(out)
	res.OK = true
	res.Hostname, res.Arch, res.OS, res.Driver = slow.Hostname, slow.Arch, slow.OS, slow.Driver
	if len(fast.GPUs) > 0 {
		res.GpuModel = fmt.Sprintf("%s x%d", fast.GPUs[0].Name, len(fast.GPUs))
	}
	return res, nil
}

func computeRunOnClient(ctx context.Context, client *ssh.SSHClient, script string) (string, error) {
	session, err := client.Client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out := &computeLimitedBuffer{max: computeSSHMaxOutput}
	session.Stdout, session.Stderr = out, out
	session.Stdin = strings.NewReader(script)
	done := make(chan error, 1)
	go func() { done <- session.Run("sh -s") }()
	select {
	case err = <-done:
		return out.buf.String(), err
	case <-ctx.Done():
		_ = session.Close()
		return out.buf.String(), errors.New("ssh command timeout")
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

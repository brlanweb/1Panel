package service

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
)

// ---- topology ------------------------------------------------------------

func computeLinks() ([]model.ComputeFabricLink, error) {
	return repo.ComputeLinkRepo.List(repo.WithComputeOrder("domain asc, id asc"))
}

func (u *ComputeService) Topology() ([]dto.ComputeFabricDomain, error) {
	links, err := computeLinks()
	if err != nil {
		return nil, err
	}
	byDomain := map[string]*dto.ComputeFabricDomain{}
	order := []string{}
	for _, l := range links {
		name := l.Domain
		if name == "" {
			name = "fd-unassigned"
		}
		d := byDomain[name]
		if d == nil {
			d = &dto.ComputeFabricDomain{Name: name, Healthy: true}
			byDomain[name] = d
			order = append(order, name)
		}
		d.Links = append(d.Links, l)
		if l.State != "ACTIVE" {
			d.Healthy = false
		}
	}
	res := make([]dto.ComputeFabricDomain, 0, len(order))
	for _, name := range order {
		d := byDomain[name]
		seen := map[uint]bool{}
		for _, l := range d.Links {
			seen[l.HostAID], seen[l.HostBID] = true, true
		}
		d.HostIDs = computeSortedIDs(seen)
		for _, id := range d.HostIDs {
			for _, r := range computeState.hostRealtime(id).Rdma {
				if r.ErrDelta > 0 {
					d.Healthy = false
				}
			}
		}
		res = append(res, *d)
	}
	return res, nil
}

func (u *ComputeService) CreateLink(req dto.ComputeLinkReq) error {
	if req.HostAID == req.HostBID {
		return buserr.WithDetail("ErrInvalidParams", "hostB", nil)
	}
	a, b := req.HostAID, req.HostBID
	l := model.ComputeFabricLink{HostAID: a, HostBID: b, IfaceA: req.IfaceA, IfaceB: req.IfaceB, IPA: req.IPA, IPB: req.IPB, Source: "manual", State: "ACTIVE"}
	if a > b {
		l.HostAID, l.HostBID, l.IfaceA, l.IfaceB, l.IPA, l.IPB = b, a, req.IfaceB, req.IfaceA, req.IPB, req.IPA
	}
	if err := repo.ComputeLinkRepo.Create(&l); err != nil {
		return err
	}
	hosts, _ := repo.ComputeHostRepo.List()
	computeRebuildDomains(hosts)
	return nil
}

func (u *ComputeService) DeleteLink(id uint) error {
	if err := repo.ComputeLinkRepo.Delete(repo.WithByID(id)); err != nil {
		return err
	}
	hosts, _ := repo.ComputeHostRepo.List()
	computeRebuildDomains(hosts)
	return nil
}

// ---- runtimes / profiles --------------------------------------------------

func (u *ComputeService) ListRuntimes() ([]model.ComputeRuntime, error) {
	return repo.ComputeRuntimeRepo.List(repo.WithComputeOrder("builtin desc, name asc"))
}

func (u *ComputeService) SaveRuntime(req dto.ComputeRuntimeReq) error {
	if req.Backend == "" {
		req.Backend = model.ComputeBackendScript
	}
	if req.Backend == model.ComputeBackendVllm {
		// Deployed through AI → vLLM of the host: single host, no scripts.
		req.MultiNode, req.LaunchTemplate, req.StopTemplate = false, "", ""
		if req.Engine != "vllm" {
			return buserr.WithDetail("ErrInvalidParams", "backend vllm needs engine vllm", nil)
		}
	} else {
		if strings.TrimSpace(req.LaunchTemplate) == "" || strings.TrimSpace(req.StopTemplate) == "" {
			return buserr.WithDetail("ErrInvalidParams", "launchTemplate / stopTemplate", nil)
		}
		for _, tpl := range []string{req.LaunchTemplate, req.StopTemplate} {
			if _, err := computeRender("check", tpl, computeTplVars{Env: map[string]string{}}); err != nil {
				return buserr.WithDetail("ErrInvalidParams", err.Error(), nil)
			}
		}
	}
	if req.HealthPath == "" {
		req.HealthPath = "/health"
	}
	if req.MetricsPath == "" {
		req.MetricsPath = "/metrics"
	}
	if req.Device == "" {
		req.Device = computeKindGPU
	}
	if dup, err := repo.ComputeRuntimeRepo.Get(repo.WithByName(req.Name)); err == nil && dup.ID != req.ID {
		return buserr.New("ErrRecordExist")
	}
	if req.ID == 0 {
		rt := model.ComputeRuntime{Name: req.Name, Engine: req.Engine, Device: req.Device, Backend: req.Backend, Image: req.Image, MultiNode: req.MultiNode,
			LaunchTemplate: req.LaunchTemplate, StopTemplate: req.StopTemplate, HealthPath: req.HealthPath,
			MetricsPath: req.MetricsPath, Remark: req.Remark}
		return repo.ComputeRuntimeRepo.Create(&rt)
	}
	return repo.ComputeRuntimeRepo.Update(req.ID, map[string]interface{}{"name": req.Name, "engine": req.Engine, "device": req.Device, "backend": req.Backend, "image": req.Image,
		"multi_node": req.MultiNode, "launch_template": req.LaunchTemplate, "stop_template": req.StopTemplate,
		"health_path": req.HealthPath, "metrics_path": req.MetricsPath, "remark": req.Remark})
}

func (u *ComputeService) DeleteRuntime(id uint) error {
	if n, _ := repo.ComputeProfileRepo.Count(repo.WithComputeField("runtime_id", id)); n > 0 {
		return buserr.WithDetail("ErrComputeInUse", "profile", nil)
	}
	return repo.ComputeRuntimeRepo.Delete(repo.WithByID(id))
}

func (u *ComputeService) ListProfiles() ([]model.ComputeProfile, error) {
	return repo.ComputeProfileRepo.List(repo.WithComputeOrder("served_name asc, name asc"))
}

func (u *ComputeService) SaveProfile(req dto.ComputeProfileReq) error {
	hosts := computeHostMap()
	var ids []uint
	seen := map[uint]bool{}
	for _, id := range req.HostIDs {
		if _, ok := hosts[id]; !ok {
			return buserr.WithDetail("ErrInvalidParams", fmt.Sprintf("host #%d", id), nil)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	// Multi-host plans are defined by their hosts (head first); a
	// single-host plan with several hosts places replicas on them.
	if req.NodeCount <= 0 {
		req.NodeCount = 1
	}
	if req.NodeCount > 1 && len(ids) > 0 && len(ids) != req.NodeCount {
		return buserr.WithDetail("ErrInvalidParams", fmt.Sprintf("a %d-host plan needs exactly %d hosts", req.NodeCount, req.NodeCount), nil)
	}
	var rt model.ComputeRuntime
	var err error
	if req.RuntimeID == 0 {
		rt, err = computeAutoRuntime(ids, req.NodeCount, hosts)
		if err != nil {
			return buserr.WithDetail("ErrInvalidParams", err.Error(), nil)
		}
		req.RuntimeID = rt.ID
	} else if rt, err = repo.ComputeRuntimeRepo.Get(repo.WithByID(req.RuntimeID)); err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	if req.NodeCount > 1 && !rt.MultiNode {
		return buserr.WithDetail("ErrInvalidParams", "runtime is single node", nil)
	}
	if req.NodeCount > 1 || req.MaxPerHost <= 0 {
		req.MaxPerHost = 1
	}
	if req.MaxPerHost > 1 && req.MinMemGB <= 0 {
		return buserr.WithDetail("ErrInvalidParams", "several replicas per host need the memory per replica (min memory)", nil)
	}
	if req.Port == 0 {
		req.Port = 8000
	}
	if req.StartTimeoutSec <= 0 {
		req.StartTimeoutSec = 1800
	}
	req.ModelRepo = strings.TrimSpace(req.ModelRepo)
	req.ModelPath = strings.TrimSpace(req.ModelPath)
	if dup, err := repo.ComputeProfileRepo.Get(repo.WithByName(req.Name)); err == nil && dup.ID != req.ID {
		return buserr.New("ErrRecordExist")
	}
	hostIDs := computeJoinIDs(ids)
	vars := map[string]interface{}{"name": req.Name, "runtime_id": req.RuntimeID, "served_name": req.ServedName,
		"model_repo": req.ModelRepo, "model_path": req.ModelPath, "host_ids": hostIDs, "node_count": req.NodeCount,
		"max_per_host": req.MaxPerHost, "need_fabric": req.NeedFabric, "min_mem_gb": req.MinMemGB, "port": req.Port,
		"image": strings.TrimSpace(req.Image), "extra_args": req.ExtraArgs, "env": req.Env,
		"start_timeout_sec": req.StartTimeoutSec, "remark": req.Remark}
	if req.ID == 0 {
		p := model.ComputeProfile{Name: req.Name, RuntimeID: req.RuntimeID, ServedName: req.ServedName, ModelRepo: req.ModelRepo,
			ModelPath: req.ModelPath, HostIDs: hostIDs, NodeCount: req.NodeCount, MaxPerHost: req.MaxPerHost,
			NeedFabric: req.NeedFabric, MinMemGB: req.MinMemGB, Port: req.Port, Image: strings.TrimSpace(req.Image),
			ExtraArgs: req.ExtraArgs, Env: req.Env, StartTimeoutSec: req.StartTimeoutSec, Remark: req.Remark}
		return repo.ComputeProfileRepo.Create(&p)
	}
	return repo.ComputeProfileRepo.Update(req.ID, vars)
}

func (u *ComputeService) DeleteProfile(id uint) error {
	if n, _ := repo.ComputeInstanceRepo.Count(repo.WithComputeField("profile_id", id)); n > 0 {
		return buserr.WithDetail("ErrComputeInUse", "instance", nil)
	}
	return repo.ComputeProfileRepo.Delete(repo.WithByID(id))
}

// ---- instances ------------------------------------------------------------

func computeInstanceKey(in model.ComputeInstance) string {
	if in.APIKeyCipher == "" {
		return ""
	}
	k, err := encrypt.StringDecrypt(in.APIKeyCipher)
	if err != nil {
		return ""
	}
	return k
}

func (u *ComputeService) ListInstances() ([]dto.ComputeInstanceView, error) {
	insts, err := repo.ComputeInstanceRepo.List(repo.WithComputeOrder("served_name asc, name asc"))
	if err != nil {
		return nil, err
	}
	hosts := computeHostMap()
	profiles, _ := repo.ComputeProfileRepo.List()
	pname := map[uint]string{}
	for _, p := range profiles {
		pname[p.ID] = p.Name
	}
	res := make([]dto.ComputeInstanceView, 0, len(insts))
	for _, in := range insts {
		res = append(res, dto.ComputeInstanceView{ComputeInstance: in, HostNames: computeHostNames(computeParseIDs(in.HostIDs), hosts),
			ProfileName: pname[in.ProfileID], Metrics: computeState.instMetrics(in.ID)})
	}
	return res, nil
}

// computeUpstreamModel is the model name the engine of an instance serves.
func computeUpstreamModel(in model.ComputeInstance) string {
	if in.UpstreamModel != "" {
		return in.UpstreamModel
	}
	return in.ServedName
}

func computeEndpoint(addr string, port int) string {
	return "http://" + net.JoinHostPort(addr, strconv.Itoa(port))
}

// SaveInstance registers (or edits) an endpoint by hand.
func (u *ComputeService) SaveInstance(req dto.ComputeInstanceReq) error {
	head, err := repo.ComputeHostRepo.Get(repo.WithByID(req.HeadHostID))
	if err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	ids := []uint{req.HeadHostID}
	for _, id := range req.HostIDs {
		if id != req.HeadHostID {
			ids = append(ids, id)
		}
	}
	if dup, err := repo.ComputeInstanceRepo.Get(repo.WithByName(req.Name)); err == nil && dup.ID != req.ID {
		return buserr.New("ErrRecordExist")
	}
	weight := req.Weight
	if weight <= 0 {
		weight = 100
	}
	req.ServedName = strings.TrimSpace(req.ServedName)
	upstream := strings.TrimSpace(req.UpstreamModel)
	if upstream == req.ServedName {
		upstream = ""
	}
	vars := map[string]interface{}{"name": req.Name, "served_name": req.ServedName, "upstream_model": upstream,
		"head_host_id": req.HeadHostID, "host_ids": computeJoinIDs(ids), "port": req.Port,
		"endpoint": computeEndpoint(head.Addr, req.Port), "container_name": req.ContainerName, "weight": weight}
	// ServedName / upstream are read by the router cache.
	defer computeRouterInvalidate()
	if req.APIKey != "" {
		c, err := encrypt.StringEncrypt(req.APIKey)
		if err != nil {
			return err
		}
		vars["api_key_cipher"] = c
	}
	if req.ID > 0 {
		return repo.ComputeInstanceRepo.Update(req.ID, vars)
	}
	in := model.ComputeInstance{Name: req.Name, ServedName: req.ServedName, UpstreamModel: upstream, HeadHostID: req.HeadHostID, HostIDs: computeJoinIDs(ids),
		Port: req.Port, Endpoint: computeEndpoint(head.Addr, req.Port), ContainerName: req.ContainerName, Adopted: true,
		Status: model.ComputeInstUnknown, RouteEnabled: true, Weight: weight}
	if c, ok := vars["api_key_cipher"].(string); ok {
		in.APIKeyCipher = c
	}
	return repo.ComputeInstanceRepo.Create(&in)
}

// Adopt registers an engine process discovered on a host without touching
// it. The served name defaults to the --served-model-name of the process.
func (u *ComputeService) Adopt(req dto.ComputeAdoptReq) error {
	host, err := repo.ComputeHostRepo.Get(repo.WithByID(req.HostID))
	if err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	served := req.ServedName
	if slow := computeState.hostSlow(host.ID); slow != nil {
		for _, p := range slow.Procs {
			if p.Port != req.Port {
				continue
			}
			// A worker rank serves no API on this port: registering it
			// only produces a permanently failing instance. The group is
			// adopted on its head with this host selected as a worker.
			if p.Worker {
				return buserr.New("ErrComputeAdoptWorker")
			}
			if served == "" {
				served = p.ServedName
			}
		}
	}
	if served == "" {
		return buserr.WithDetail("ErrInvalidParams", "servedName", nil)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("%s-%d", host.Name, req.Port)
	}
	return u.SaveInstance(dto.ComputeInstanceReq{Name: name, ServedName: served, HeadHostID: host.ID,
		HostIDs: req.WorkerHostIDs, Port: req.Port, APIKey: req.APIKey})
}

func (u *ComputeService) SetRoute(req dto.ComputeRouteReq) error {
	if _, err := repo.ComputeInstanceRepo.Get(repo.WithByID(req.ID)); err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	return repo.ComputeInstanceRepo.Update(req.ID, map[string]interface{}{"route_enabled": req.RouteEnabled, "weight": req.Weight})
}

func (u *ComputeService) OperateInstance(req dto.ComputeInstanceOpReq) (uint, error) {
	in, err := repo.ComputeInstanceRepo.Get(repo.WithByID(req.ID))
	if err != nil {
		return 0, buserr.New("ErrRecordNotFound")
	}
	switch req.Operation {
	case "drain":
		return 0, repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"route_enabled": false})
	case "undrain":
		return 0, repo.ComputeInstanceRepo.Update(in.ID, map[string]interface{}{"route_enabled": true})
	}
	if req.Operation == "remove" && in.Adopted && !req.Force {
		// Unregister only: the running engine is left alone.
		if err := repo.ComputeInstanceRepo.Delete(repo.WithByID(in.ID)); err != nil {
			return 0, err
		}
		computeState.dropInst(in.ID)
		return 0, nil
	}
	if in.Adopted && !req.Force {
		return 0, buserr.New("ErrComputeAdopted")
	}
	if in.ProfileID == 0 && req.Operation != "stop" && req.Operation != "remove" {
		return 0, buserr.New("ErrComputeAdopted")
	}
	return computeStartJob(computeJobSpec{Type: req.Operation, InstanceID: in.ID, ProfileID: in.ProfileID,
		ServedName: in.ServedName, HostIDs: computeParseIDs(in.HostIDs), Source: "manual"})
}

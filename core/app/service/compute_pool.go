package service

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
)

// Compute pool ("算力池"): the user facing form of compute scheduling.
//
// A pool is one served model with a scheduling policy. Creating it also
// creates, on this (master) panel only:
//   - a router key restricted to the model, and
//   - an AI → Model Services entry ("custom", OpenAI compatible) whose base
//     URL is the local model router and whose key is that router key,
// so agents and apps use the pool like any other model service while the
// router spreads requests over the replicas and the scheduler adds /
// removes replicas (one profile = one replica, e.g. two GB10 over the
// optical link).

const computePoolProvider = "custom"

type computeAgentAccount struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl"`
}

func computeLocalCall(method, path string, body, out interface{}, timeout time.Duration) error {
	return computePanelCall(model.ComputeHost{Source: model.ComputeSourceLocal, Name: "local"}, method, path, body, out, timeout)
}

// computePoolAccounts lists the local model services by id.
func computePoolAccounts() (map[uint]computeAgentAccount, error) {
	var page computeVllmPage[computeAgentAccount]
	body := map[string]interface{}{"page": 1, "pageSize": 1000, "provider": computePoolProvider}
	if err := computeLocalCall(http.MethodPost, "/api/v2/ai/accounts/search", body, &page, 20*time.Second); err != nil {
		return nil, err
	}
	res := make(map[uint]computeAgentAccount, len(page.Items))
	for _, a := range page.Items {
		res[a.ID] = a
	}
	return res, nil
}

// computeRouterLocalURL is the base URL the local agent uses to reach the
// router; computeRouterPublicURL is the one shown to external callers.
func computeRouterLocalURL(conf computeRouterConf) string {
	host, port, err := net.SplitHostPort(conf.Listen)
	if err != nil {
		return "http://127.0.0.1:18080/v1"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/v1"
}

func computeRouterPublicURL(conf computeRouterConf) string {
	host, port, err := net.SplitHostPort(conf.Listen)
	if err != nil {
		return ""
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host, _ = settingRepo.GetValueByKey("SystemIP")
		host = strings.TrimSpace(host)
		if host == "" {
			host = "<master-ip>"
		}
	}
	return "http://" + net.JoinHostPort(host, port) + "/v1"
}

// computeEnsureRouter turns the router on (default listen) when a pool is
// bound: a pool without the router has no endpoint.
func computeEnsureRouter() (computeRouterConf, error) {
	conf := computeLoadRouterConf()
	if conf.Enabled {
		return conf, nil
	}
	set, _ := (&ComputeService{}).GetRouterSetting()
	set.Enabled = true
	if set.Listen == "" {
		set.Listen = computeDefaultRouterConf().Listen
	}
	if err := (&ComputeService{}).UpdateRouterSetting(set); err != nil {
		return conf, fmt.Errorf("start the model router on %s: %w", set.Listen, err)
	}
	return computeLoadRouterConf(), nil
}

func (u *ComputeService) ListPools() ([]dto.ComputePoolView, error) {
	policies, err := repo.ComputePolicyRepo.List(repo.WithComputeOrder("priority desc, served_name asc"))
	if err != nil {
		return nil, err
	}
	insts, _ := repo.ComputeInstanceRepo.List()
	profiles, _ := repo.ComputeProfileRepo.List()
	profMap := map[uint]model.ComputeProfile{}
	for _, p := range profiles {
		profMap[p.ID] = p
	}
	accounts, accErr := computePoolAccounts()
	base := computeRouterPublicURL(computeLoadRouterConf())
	res := make([]dto.ComputePoolView, 0, len(policies))
	for _, p := range policies {
		v := dto.ComputePoolView{ComputePolicy: p, BaseURL: base}
		if prof, ok := profMap[p.ProfileID]; ok {
			v.ProfileName, v.NodeCount = prof.Name, prof.NodeCount
		}
		for _, in := range insts {
			if in.ServedName != p.ServedName || in.Status == model.ComputeInstStopped {
				continue
			}
			v.Instances++
			if m := computeState.instMetrics(in.ID); m.Healthy && in.Status == model.ComputeInstRunning {
				v.Healthy++
			}
		}
		if p.AccountID > 0 && accErr == nil {
			_, ok := accounts[p.AccountID]
			v.ServiceMissing = !ok
		}
		res = append(res, v)
	}
	return res, nil
}

func (u *ComputeService) CreatePool(req dto.ComputePoolReq) (dto.ComputePoolRes, error) {
	req.ID = 0
	req.Name = strings.TrimSpace(req.Name)
	if dup, err := repo.ComputePolicyRepo.Get(repo.WithComputeField("served_name", req.ServedName)); err == nil && dup.ID > 0 {
		return dto.ComputePoolRes{}, buserr.WithDetail("ErrInvalidParams", "model "+req.ServedName+" already has a pool", nil)
	}
	id, err := computeSavePolicy(req.ComputePolicyReq, &model.ComputePolicy{Name: req.Name})
	if err != nil {
		return dto.ComputePoolRes{}, err
	}
	res, err := u.bindPoolService(id)
	if err != nil {
		_ = repo.ComputePolicyRepo.Delete(repo.WithByID(id))
		return dto.ComputePoolRes{}, err
	}
	return res, nil
}

// BindPoolService (re)creates the router key and model service of a pool,
// e.g. for a policy created before pools existed, after the service was
// deleted in Model Services, or to rotate the key.
func (u *ComputeService) BindPoolService(id uint) (dto.ComputePoolRes, error) {
	return u.bindPoolService(id)
}

func (u *ComputeService) bindPoolService(id uint) (dto.ComputePoolRes, error) {
	p, err := repo.ComputePolicyRepo.Get(repo.WithByID(id))
	if err != nil {
		return dto.ComputePoolRes{}, buserr.New("ErrRecordNotFound")
	}
	name := p.Name
	if name == "" {
		name = p.ServedName
	}
	conf, err := computeEnsureRouter()
	if err != nil {
		return dto.ComputePoolRes{}, err
	}
	accounts, err := computePoolAccounts()
	if err != nil {
		return dto.ComputePoolRes{}, err
	}
	// A fresh key on every bind: the plain text key is only known now.
	keyName := computeTrim("pool:"+name, 64)
	if old, e := repo.ComputeConsumerRepo.Get(repo.WithByID(p.ConsumerID)); e == nil && old.ID > 0 && p.ConsumerID > 0 {
		_ = u.DeleteConsumer(old.ID)
	}
	if old, e := repo.ComputeConsumerRepo.Get(repo.WithByName(keyName)); e == nil && old.ID > 0 {
		_ = u.DeleteConsumer(old.ID)
	}
	enabled := true
	key, err := u.SaveConsumer(dto.ComputeConsumerReq{Name: keyName, AllowedModels: p.ServedName, Enabled: &enabled,
		Remark: "model service of compute pool " + name})
	if err != nil {
		return dto.ComputePoolRes{}, err
	}
	if _, ok := accounts[p.AccountID]; ok && p.AccountID > 0 {
		_ = computeLocalCall(http.MethodPost, "/api/v2/ai/accounts/delete", map[string]uint{"id": p.AccountID}, nil, 30*time.Second)
	}
	validate := false
	body := map[string]interface{}{
		"provider": computePoolProvider, "accountType": "default", "name": name, "apiKey": key.Key, "rememberApiKey": true,
		"baseURL": computeRouterLocalURL(conf), "apiType": "openai-completions", "authMode": "bearer",
		"models":      []map[string]string{{"id": p.ServedName, "name": p.ServedName}},
		"verifyModel": p.ServedName, "validateAvailability": &validate,
		"remark":      "XPanel compute pool: requests are load-balanced over the replicas of " + p.ServedName,
	}
	if err := computeLocalCall(http.MethodPost, "/api/v2/ai/accounts", body, nil, time.Minute); err != nil {
		_ = u.DeleteConsumer(key.ID)
		return dto.ComputePoolRes{}, fmt.Errorf("create model service %s: %w", name, err)
	}
	accountID := uint(0)
	if after, err := computePoolAccounts(); err == nil {
		for _, a := range after {
			if a.Name == name && a.ID > accountID {
				accountID = a.ID
			}
		}
	}
	if err := repo.ComputePolicyRepo.Update(p.ID, map[string]interface{}{"name": name, "account_id": accountID, "consumer_id": key.ID}); err != nil {
		return dto.ComputePoolRes{}, err
	}
	global.LOG.Infof("compute pool %s (%s) bound to model service #%d", name, p.ServedName, accountID)
	return dto.ComputePoolRes{ID: p.ID, AccountID: accountID, Key: key.Key}, nil
}

func (u *ComputeService) UpdatePool(req dto.ComputePoolReq) error {
	p, err := repo.ComputePolicyRepo.Get(repo.WithByID(req.ID))
	if err != nil || req.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	req.ServedName = p.ServedName // the model of a pool is fixed
	_, err = computeSavePolicy(req.ComputePolicyReq, nil)
	return err
}

func (u *ComputeService) DeletePool(req dto.ComputePoolDeleteReq) error {
	p, err := repo.ComputePolicyRepo.Get(repo.WithByID(req.ID))
	if err != nil {
		return buserr.New("ErrRecordNotFound")
	}
	if req.RemoveService && p.AccountID > 0 {
		if accounts, err := computePoolAccounts(); err == nil {
			if _, ok := accounts[p.AccountID]; ok {
				if err := computeLocalCall(http.MethodPost, "/api/v2/ai/accounts/delete", map[string]uint{"id": p.AccountID}, nil, 30*time.Second); err != nil {
					return fmt.Errorf("delete model service: %w", err)
				}
			}
		}
	}
	// Without the policy the router still serves the model by name, so a
	// kept model service keeps working with its key; only drop the key
	// together with the service.
	if req.RemoveService && p.ConsumerID > 0 {
		_ = u.DeleteConsumer(p.ConsumerID)
	}
	return repo.ComputePolicyRepo.Delete(repo.WithByID(p.ID))
}

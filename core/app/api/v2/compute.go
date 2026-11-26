package v2

import (
	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// Compute scheduling ("算力调度"). Read endpoints are GET or plain POST
// without @x-panel-log; every state changing endpoint is logged.

func computeBind[T any](c *gin.Context) (T, bool) {
	var req T
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return req, false
	}
	return req, true
}

func computeReply(c *gin.Context, data interface{}, err error) {
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	if data == nil {
		helper.Success(c)
		return
	}
	helper.SuccessWithData(c, data)
}

// @Tags Compute
// @Summary Compute overview
// @Success 200 {object} dto.ComputeOverview
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/overview [get]
func (b *BaseApi) ComputeOverview(c *gin.Context) {
	data, err := computeService.Overview()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Heartbeat of a visible compute page (switches to the fast cadence for 30s)
// @Accept json
// @Param request body dto.ComputeWatchReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/watch [post]
func (b *BaseApi) ComputeWatch(c *gin.Context) {
	req, ok := computeBind[dto.ComputeWatchReq](c)
	if !ok {
		return
	}
	computeService.Watch(req.Fast)
	helper.Success(c)
}

// @Tags Compute
// @Summary List compute hosts with realtime samples
// @Accept json
// @Param request body dto.SearchWithPage true "request"
// @Success 200 {array} dto.ComputeHostView
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/hosts/search [post]
func (b *BaseApi) ComputeListHosts(c *gin.Context) {
	var req struct {
		Info string `json:"info"`
	}
	_ = c.ShouldBindJSON(&req)
	data, err := computeService.ListHosts(req.Info)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Hosts of a source (terminal / node / local) that can be imported
// @Accept json
// @Param request body object true "request"
// @Success 200 {array} dto.ComputeHostCandidate
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/hosts/candidates [post]
func (b *BaseApi) ComputeHostCandidates(c *gin.Context) {
	var req struct {
		Source string `json:"source"`
	}
	_ = c.ShouldBindJSON(&req)
	data, err := computeService.HostCandidates(req.Source)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Bring existing terminal hosts / cluster nodes under compute scheduling
// @Accept json
// @Param request body dto.ComputeHostImportReq true "request"
// @Success 200 {object} dto.ComputeHostImportRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/hosts/import [post]
// @x-panel-log {"bodyKeys":["source"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"导入算力主机 [source]","formatEN":"import compute hosts [source]"}
func (b *BaseApi) ComputeImportHosts(c *gin.Context) {
	req, ok := computeBind[dto.ComputeHostImportReq](c)
	if !ok {
		return
	}
	data, err := computeService.ImportHosts(req)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Update compute host
// @Accept json
// @Param request body dto.ComputeHostReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/hosts/update [post]
// @x-panel-log {"bodyKeys":["name"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"修改算力主机 [name]","formatEN":"update compute host [name]"}
func (b *BaseApi) ComputeUpdateHost(c *gin.Context) {
	req, ok := computeBind[dto.ComputeHostReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.UpdateHost(req))
}

// @Tags Compute
// @Summary Remove a host from compute scheduling (the host itself is untouched)
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/hosts/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_hosts","output_column":"name","output_value":"name"}],"formatZH":"删除算力主机 [name]","formatEN":"delete compute host [name]"}
func (b *BaseApi) ComputeDeleteHost(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.DeleteHost(req.ID))
}

// @Tags Compute
// @Summary Cordon / uncordon a compute host
// @Accept json
// @Param request body dto.ComputeSchedulableReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/hosts/schedulable [post]
// @x-panel-log {"bodyKeys":["id","schedulable"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_hosts","output_column":"name","output_value":"name"}],"formatZH":"设置算力主机 [name] 可调度 [schedulable]","formatEN":"set compute host [name] schedulable [schedulable]"}
func (b *BaseApi) ComputeSetSchedulable(c *gin.Context) {
	req, ok := computeBind[dto.ComputeSchedulableReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.SetSchedulable(req))
}

// @Tags Compute
// @Summary Collect a compute host now
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/hosts/refresh [post]
func (b *BaseApi) ComputeRefreshHost(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.RefreshHost(req.ID))
}

// @Tags Compute
// @Summary Fabric domains and links
// @Success 200 {array} dto.ComputeFabricDomain
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/topology [get]
func (b *BaseApi) ComputeTopology(c *gin.Context) {
	data, err := computeService.Topology()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Add a fabric link by hand
// @Accept json
// @Param request body dto.ComputeLinkReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/topology/link [post]
// @x-panel-log {"bodyKeys":["hostAId","hostBId"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"添加互联链路 [hostAId]-[hostBId]","formatEN":"add fabric link [hostAId]-[hostBId]"}
func (b *BaseApi) ComputeCreateLink(c *gin.Context) {
	req, ok := computeBind[dto.ComputeLinkReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.CreateLink(req))
}

// @Tags Compute
// @Summary Delete a fabric link
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/topology/link/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"删除互联链路 [id]","formatEN":"delete fabric link [id]"}
func (b *BaseApi) ComputeDeleteLink(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.DeleteLink(req.ID))
}

// @Tags Compute
// @Summary List runtimes
// @Success 200 {array} model.ComputeRuntime
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/runtimes [get]
func (b *BaseApi) ComputeListRuntimes(c *gin.Context) {
	data, err := computeService.ListRuntimes()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Create or update a runtime
// @Accept json
// @Param request body dto.ComputeRuntimeReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/runtimes [post]
// @x-panel-log {"bodyKeys":["name"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"保存推理引擎模板 [name]","formatEN":"save runtime [name]"}
func (b *BaseApi) ComputeSaveRuntime(c *gin.Context) {
	req, ok := computeBind[dto.ComputeRuntimeReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.SaveRuntime(req))
}

// @Tags Compute
// @Summary Delete a runtime
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/runtimes/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_runtimes","output_column":"name","output_value":"name"}],"formatZH":"删除推理引擎模板 [name]","formatEN":"delete runtime [name]"}
func (b *BaseApi) ComputeDeleteRuntime(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.DeleteRuntime(req.ID))
}

// @Tags Compute
// @Summary List model profiles
// @Success 200 {array} model.ComputeProfile
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/profiles [get]
func (b *BaseApi) ComputeListProfiles(c *gin.Context) {
	data, err := computeService.ListProfiles()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Create or update a model profile
// @Accept json
// @Param request body dto.ComputeProfileReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/profiles [post]
// @x-panel-log {"bodyKeys":["name","servedName"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"保存模型配置 [name] [servedName]","formatEN":"save model profile [name] [servedName]"}
func (b *BaseApi) ComputeSaveProfile(c *gin.Context) {
	req, ok := computeBind[dto.ComputeProfileReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.SaveProfile(req))
}

// @Tags Compute
// @Summary Delete a model profile
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/profiles/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_profiles","output_column":"name","output_value":"name"}],"formatZH":"删除模型配置 [name]","formatEN":"delete model profile [name]"}
func (b *BaseApi) ComputeDeleteProfile(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.DeleteProfile(req.ID))
}

// @Tags Compute
// @Summary List serving instances with live metrics
// @Success 200 {array} dto.ComputeInstanceView
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/instances [get]
func (b *BaseApi) ComputeListInstances(c *gin.Context) {
	data, err := computeService.ListInstances()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Register or edit an instance by hand
// @Accept json
// @Param request body dto.ComputeInstanceReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/instances [post]
// @x-panel-log {"bodyKeys":["name","servedName"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"登记推理实例 [name] [servedName]","formatEN":"register instance [name] [servedName]"}
func (b *BaseApi) ComputeSaveInstance(c *gin.Context) {
	req, ok := computeBind[dto.ComputeInstanceReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.SaveInstance(req))
}

// @Tags Compute
// @Summary Adopt a discovered engine process (read only, nothing restarted)
// @Accept json
// @Param request body dto.ComputeAdoptReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/instances/adopt [post]
// @x-panel-log {"bodyKeys":["hostId","port"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"hostId","isList":false,"db":"compute_hosts","output_column":"name","output_value":"name"}],"formatZH":"接管推理实例 [name]:[port]","formatEN":"adopt instance [name]:[port]"}
func (b *BaseApi) ComputeAdopt(c *gin.Context) {
	req, ok := computeBind[dto.ComputeAdoptReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.Adopt(req))
}

// @Tags Compute
// @Summary Set routing switch / weight of an instance
// @Accept json
// @Param request body dto.ComputeRouteReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/instances/route [post]
// @x-panel-log {"bodyKeys":["id","routeEnabled","weight"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_instances","output_column":"name","output_value":"name"}],"formatZH":"设置实例 [name] 路由 [routeEnabled] 权重 [weight]","formatEN":"set instance [name] route [routeEnabled] weight [weight]"}
func (b *BaseApi) ComputeSetRoute(c *gin.Context) {
	req, ok := computeBind[dto.ComputeRouteReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.SetRoute(req))
}

// @Tags Compute
// @Summary Operate an instance (stop/start/restart/remove/drain/undrain)
// @Accept json
// @Param request body dto.ComputeInstanceOpReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/instances/operate [post]
// @x-panel-log {"bodyKeys":["id","operation"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_instances","output_column":"name","output_value":"name"}],"formatZH":"推理实例 [name] 执行 [operation]","formatEN":"instance [name] [operation]"}
func (b *BaseApi) ComputeOperateInstance(c *gin.Context) {
	req, ok := computeBind[dto.ComputeInstanceOpReq](c)
	if !ok {
		return
	}
	id, err := computeService.OperateInstance(req)
	computeReply(c, map[string]uint{"jobId": id}, err)
}

// @Tags Compute
// @Summary Precheck and deploy a profile (dryRun = precheck only)
// @Accept json
// @Param request body dto.ComputeDeployReq true "request"
// @Success 200 {object} dto.ComputePrecheckRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/deploy [post]
// @x-panel-log {"bodyKeys":["profileId","dryRun"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"profileId","isList":false,"db":"compute_profiles","output_column":"name","output_value":"name"}],"formatZH":"部署模型 [name] (预检 [dryRun])","formatEN":"deploy model [name] (dry run [dryRun])"}
func (b *BaseApi) ComputeDeploy(c *gin.Context) {
	req, ok := computeBind[dto.ComputeDeployReq](c)
	if !ok {
		return
	}
	data, err := computeService.Deploy(req)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Precheck and switch the model of an instance (dryRun = precheck only)
// @Accept json
// @Param request body dto.ComputeSwitchReq true "request"
// @Success 200 {object} dto.ComputePrecheckRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/switch [post]
// @x-panel-log {"bodyKeys":["instanceId","profileId","dryRun"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"instanceId","isList":false,"db":"compute_instances","output_column":"name","output_value":"name"}],"formatZH":"切换实例 [name] 模型 (预检 [dryRun])","formatEN":"switch model of instance [name] (dry run [dryRun])"}
func (b *BaseApi) ComputeSwitch(c *gin.Context) {
	req, ok := computeBind[dto.ComputeSwitchReq](c)
	if !ok {
		return
	}
	data, err := computeService.Switch(req)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Page jobs
// @Accept json
// @Param request body dto.SearchWithPage true "request"
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/jobs/search [post]
func (b *BaseApi) ComputePageJobs(c *gin.Context) {
	req, ok := computeBind[dto.SearchWithPage](c)
	if !ok {
		return
	}
	total, items, err := computeService.PageJobs(req)
	computeReply(c, dto.PageResult{Total: total, Items: items}, err)
}

// @Tags Compute
// @Summary Get a job
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200 {object} model.ComputeJob
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/jobs/get [post]
func (b *BaseApi) ComputeGetJob(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	data, err := computeService.GetJob(req.ID)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary List router consumers
// @Success 200 {array} model.ComputeConsumer
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/consumers [get]
func (b *BaseApi) ComputeListConsumers(c *gin.Context) {
	data, err := computeService.ListConsumers()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Create or update a router consumer (the key is returned once on create)
// @Accept json
// @Param request body dto.ComputeConsumerReq true "request"
// @Success 200 {object} dto.ComputeConsumerKeyRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/consumers [post]
// @x-panel-log {"bodyKeys":["name"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"保存接入方 [name]","formatEN":"save consumer [name]"}
func (b *BaseApi) ComputeSaveConsumer(c *gin.Context) {
	req, ok := computeBind[dto.ComputeConsumerReq](c)
	if !ok {
		return
	}
	data, err := computeService.SaveConsumer(req)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Reset the key of a router consumer
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200 {object} dto.ComputeConsumerKeyRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/consumers/reset [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_consumers","output_column":"name","output_value":"name"}],"formatZH":"重置接入方 [name] 密钥","formatEN":"reset key of consumer [name]"}
func (b *BaseApi) ComputeResetConsumer(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	data, err := computeService.ResetConsumerKey(req.ID)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Delete a router consumer
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/consumers/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_consumers","output_column":"name","output_value":"name"}],"formatZH":"删除接入方 [name]","formatEN":"delete consumer [name]"}
func (b *BaseApi) ComputeDeleteConsumer(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.DeleteConsumer(req.ID))
}

// @Tags Compute
// @Summary Router and scheduler settings
// @Success 200 {object} dto.ComputeRouterSetting
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/setting [get]
func (b *BaseApi) ComputeGetSetting(c *gin.Context) {
	data, err := computeService.GetRouterSetting()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Update router and scheduler settings
// @Accept json
// @Param request body dto.ComputeRouterSetting true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/setting [post]
// @x-panel-log {"bodyKeys":["enabled","listen","schedulerPaused"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"修改模型路由设置 启用 [enabled] 监听 [listen] 暂停自动调度 [schedulerPaused]","formatEN":"update model router enabled [enabled] listen [listen] scheduler paused [schedulerPaused]"}
func (b *BaseApi) ComputeUpdateSetting(c *gin.Context) {
	req, ok := computeBind[dto.ComputeRouterSetting](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.UpdateRouterSetting(req))
}

// @Tags Compute
// @Summary List scheduling policies
// @Success 200 {array} model.ComputePolicy
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/policies [get]
func (b *BaseApi) ComputeListPolicies(c *gin.Context) {
	data, err := computeService.ListPolicies()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Create or update a scheduling policy
// @Accept json
// @Param request body dto.ComputePolicyReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/policies [post]
// @x-panel-log {"bodyKeys":["servedName","mode"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"保存调度策略 [servedName] 模式 [mode]","formatEN":"save scheduling policy [servedName] mode [mode]"}
func (b *BaseApi) ComputeSavePolicy(c *gin.Context) {
	req, ok := computeBind[dto.ComputePolicyReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.SavePolicy(req))
}

// @Tags Compute
// @Summary Delete a scheduling policy
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/policies/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_policies","output_column":"served_name","output_value":"name"}],"formatZH":"删除调度策略 [name]","formatEN":"delete scheduling policy [name]"}
func (b *BaseApi) ComputeDeletePolicy(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.DeletePolicy(req.ID))
}

// @Tags Compute
// @Summary Current scheduler suggestions
// @Success 200 {array} dto.ComputeSuggestion
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/suggestions [get]
func (b *BaseApi) ComputeSuggestions(c *gin.Context) {
	helper.SuccessWithData(c, computeService.Suggestions())
}

// @Tags Compute
// @Summary Apply or dismiss a suggestion
// @Accept json
// @Param request body dto.ComputeSuggestionOpReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/suggestions/operate [post]
// @x-panel-log {"bodyKeys":["id","operation"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"调度建议 [id] 执行 [operation]","formatEN":"scheduler suggestion [id] [operation]"}
func (b *BaseApi) ComputeOperateSuggestion(c *gin.Context) {
	req, ok := computeBind[dto.ComputeSuggestionOpReq](c)
	if !ok {
		return
	}
	id, err := computeService.OperateSuggestion(req)
	computeReply(c, map[string]uint{"jobId": id}, err)
}

// @Tags Compute
// @Summary List compute pools
// @Success 200 {array} dto.ComputePoolView
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/pools [get]
func (b *BaseApi) ComputeListPools(c *gin.Context) {
	data, err := computeService.ListPools()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Create a compute pool (policy + router key + model service)
// @Accept json
// @Param request body dto.ComputePoolReq true "request"
// @Success 200 {object} dto.ComputePoolRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/pools [post]
// @x-panel-log {"bodyKeys":["name","servedName"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"创建算力池 [name] 模型 [servedName]","formatEN":"create compute pool [name] model [servedName]"}
func (b *BaseApi) ComputeCreatePool(c *gin.Context) {
	req, ok := computeBind[dto.ComputePoolReq](c)
	if !ok {
		return
	}
	data, err := computeService.CreatePool(req)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Update the scheduling strategy of a compute pool
// @Accept json
// @Param request body dto.ComputePoolReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/pools/update [post]
// @x-panel-log {"bodyKeys":["name","mode"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"更新算力池 [name] 调度模式 [mode]","formatEN":"update compute pool [name] mode [mode]"}
func (b *BaseApi) ComputeUpdatePool(c *gin.Context) {
	req, ok := computeBind[dto.ComputePoolReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.UpdatePool(req))
}

// @Tags Compute
// @Summary Delete a compute pool
// @Accept json
// @Param request body dto.ComputePoolDeleteReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/pools/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_policies","output_column":"served_name","output_value":"name"}],"formatZH":"删除算力池 [name]","formatEN":"delete compute pool [name]"}
func (b *BaseApi) ComputeDeletePool(c *gin.Context) {
	req, ok := computeBind[dto.ComputePoolDeleteReq](c)
	if !ok {
		return
	}
	computeReply(c, nil, computeService.DeletePool(req))
}

// @Tags Compute
// @Summary (Re)create the model service and router key of a compute pool
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200 {object} dto.ComputePoolRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/pools/bind [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"compute_policies","output_column":"served_name","output_value":"name"}],"formatZH":"重建算力池 [name] 的模型服务与密钥","formatEN":"rebind model service and key of compute pool [name]"}
func (b *BaseApi) ComputeBindPool(c *gin.Context) {
	req, ok := computeBind[dto.OperateByID](c)
	if !ok {
		return
	}
	data, err := computeService.BindPoolService(req.ID)
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Prepared models of all panel hosts (AI → vLLM → Models), by repo
// @Success 200 {object} dto.ComputeModelList
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/models [get]
func (b *BaseApi) ComputeListModels(c *gin.Context) {
	data, err := computeService.ListModels()
	computeReply(c, data, err)
}

// @Tags Compute
// @Summary Page model router call logs (memory only, retention window)
// @Accept json
// @Param request body dto.ComputeCallLogSearch true "request"
// @Success 200 {object} dto.ComputeCallLogPage
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/logs/search [post]
func (b *BaseApi) ComputePageCallLogs(c *gin.Context) {
	req, ok := computeBind[dto.ComputeCallLogSearch](c)
	if !ok {
		return
	}
	helper.SuccessWithData(c, computeService.PageCallLogs(req))
}

// @Tags Compute
// @Summary Model call analytics (from the in-memory call logs)
// @Accept json
// @Param request body dto.ComputeAnalyticsReq true "request"
// @Success 200 {object} dto.ComputeAnalytics
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/compute/analytics [post]
func (b *BaseApi) ComputeAnalytics(c *gin.Context) {
	req, ok := computeBind[dto.ComputeAnalyticsReq](c)
	if !ok {
		return
	}
	helper.SuccessWithData(c, computeService.Analytics(req))
}

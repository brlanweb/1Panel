package v2

import (
	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags Proxy Node
// @Summary Load proxy node status (subscription, nodes, test results)
// @Success 200 {object} dto.ProxyNodeStatus
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/proxy-node [get]
func (b *BaseApi) LoadProxyNodeStatus(c *gin.Context) {
	helper.SuccessWithData(c, proxyNodeService.Status())
}

// @Tags Proxy Node
// @Summary Save a Clash subscription, download it and start testing all nodes
// @Accept json
// @Param request body dto.ProxyNodeSave true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/proxy-node/save [post]
// @x-panel-log {"bodyKeys":[],"paramKeys":[],"BeforeFunctions":[],"formatZH":"保存代理节点订阅","formatEN":"save proxy node subscription"}
func (b *BaseApi) SaveProxyNode(c *gin.Context) {
	var req dto.ProxyNodeSave
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := proxyNodeService.Save(req); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Proxy Node
// @Summary Re-download the subscription
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/proxy-node/refresh [post]
func (b *BaseApi) RefreshProxyNode(c *gin.Context) {
	if err := proxyNodeService.Refresh(); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Proxy Node
// @Summary Select the active node
// @Accept json
// @Param request body dto.ProxyNodeSelect true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/proxy-node/select [post]
// @x-panel-log {"bodyKeys":["name"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"切换代理节点 [name]","formatEN":"switch proxy node [name]"}
func (b *BaseApi) SelectProxyNode(c *gin.Context) {
	var req dto.ProxyNodeSelect
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := proxyNodeService.Select(req.Name); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Proxy Node
// @Summary Test latency and speed of all nodes in the background
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/proxy-node/test [post]
func (b *BaseApi) TestProxyNodes(c *gin.Context) {
	if err := proxyNodeService.Test(); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Proxy Node
// @Summary Start or stop the proxy node container
// @Accept json
// @Param request body dto.ProxyNodeOperate true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/proxy-node/operate [post]
func (b *BaseApi) OperateProxyNode(c *gin.Context) {
	var req dto.ProxyNodeOperate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := proxyNodeService.Operate(req.Operate); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

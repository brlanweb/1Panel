package v2

import (
	"net"
	"net/http"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/gin-gonic/gin"
)

// fromLocalSocket reports whether the request came in over the master's
// unix socket (i.e. from this panel's own core process).
func fromLocalSocket(c *gin.Context) bool {
	local, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr)
	return global.IsMaster && ok && local.Network() == "unix"
}

// @Tags Alert
// @Summary List per-target states of a cluster alert rule
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200 {array} dto.ClusterAlertTargetStateDTO
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /alert/cluster/states [post]
func (b *BaseApi) ListClusterAlertStates(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	data, err := service.ListClusterTargetStates(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, data)
}

// ListClusterAlertRules hands the enabled cluster resource rules to the
// master core sampler. Internal: unix socket only.
func (b *BaseApi) ListClusterAlertRules(c *gin.Context) {
	if !fromLocalSocket(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	data, err := service.ListClusterResourceRules()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, data)
}

// ReportClusterAlert receives cluster resource check results from the
// master core sampler. Internal: unix socket only.
func (b *BaseApi) ReportClusterAlert(c *gin.Context) {
	if !fromLocalSocket(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var req dto.ClusterAlertReport
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := service.ProcessClusterChecks(req.Checks); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Alert
// @Summary Summary of cluster alert targets for the home page
// @Success 200 {object} dto.ClusterAlertSummary
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /alert/cluster/summary [get]
func (b *BaseApi) ClusterAlertSummary(c *gin.Context) {
	data, err := service.ClusterAlertSummary(20)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, data)
}

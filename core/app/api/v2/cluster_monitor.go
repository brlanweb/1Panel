package v2

import (
	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags Cluster Monitor
// @Summary Realtime resource sample of every node (CPU, network speed)
// @Success 200 {object} dto.ClusterMonitorRealtimeRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/monitor/realtime [get]
func (b *BaseApi) LoadClusterMonitorRealtime(c *gin.Context) {
	data, err := clusterMonitorService.Realtime()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, data)
}

// @Tags Cluster Monitor
// @Summary Cached static resource snapshot of every node (memory, disks, IPs, risks)
// @Accept json
// @Param request body dto.ClusterMonitorStaticReq true "request"
// @Success 200 {object} dto.ClusterMonitorStaticRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/monitor/static [post]
func (b *BaseApi) LoadClusterMonitorStatic(c *gin.Context) {
	var req dto.ClusterMonitorStaticReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	data, err := clusterMonitorService.Static(req.Force, req.WithHW)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, data)
}

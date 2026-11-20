package v2

import (
	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags Cluster WAF
// @Summary Get centralized WAF overview (website counts only; no traffic or interception numbers, see DESIGN.md section 12.3 B)
// @Success 200 {object} dto.ClusterWafOverview
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/waf/overview [get]
func (b *BaseApi) GetClusterWafOverview(c *gin.Context) {
	overview, err := clusterWafService.Overview()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, overview)
}

// @Tags Cluster WAF
// @Summary List websites across cluster nodes with their (read-only, schema-pending) WAF state
// @Accept json
// @Param request body dto.ClusterWafWebsiteSearch true "request"
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/waf/websites/search [post]
func (b *BaseApi) SearchClusterWafWebsites(c *gin.Context) {
	var req dto.ClusterWafWebsiteSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	total, list, err := clusterWafService.Websites(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, dto.PageResult{
		Total: total,
		Items: list,
	})
}

package v2

import (
	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags Cluster Node
// @Summary Search cluster nodes
// @Accept json
// @Param request body dto.ClusterNodeSearch true "request"
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes/search [post]
func (b *BaseApi) SearchClusterNode(c *gin.Context) {
	var req dto.ClusterNodeSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	total, list, err := clusterNodeService.SearchWithPage(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, dto.PageResult{
		Total: total,
		Items: list,
	})
}

// @Tags Cluster Node
// @Summary Create cluster node
// @Accept json
// @Param request body dto.ClusterNodeCreate true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes [post]
// @x-panel-log {"bodyKeys":["name"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"添加集群节点 [name]","formatEN":"add cluster node [name]"}
func (b *BaseApi) CreateClusterNode(c *gin.Context) {
	var req dto.ClusterNodeCreate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterNodeService.Create(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster Node
// @Summary Update cluster node
// @Accept json
// @Param request body dto.ClusterNodeUpdate true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes/update [post]
// @x-panel-log {"bodyKeys":["name"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"更新集群节点 [name]","formatEN":"update cluster node [name]"}
func (b *BaseApi) UpdateClusterNode(c *gin.Context) {
	var req dto.ClusterNodeUpdate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterNodeService.Update(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster Node
// @Summary Delete cluster node
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"cluster_nodes","output_column":"name","output_value":"name"}],"formatZH":"删除集群节点 [name]","formatEN":"delete cluster node [name]"}
func (b *BaseApi) DeleteClusterNode(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterNodeService.Delete(req.ID); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster Node
// @Summary Test cluster node connection
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200 {object} dto.ClusterNodeInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes/test [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"id","isList":false,"db":"cluster_nodes","output_column":"name","output_value":"name"}],"formatZH":"测试集群节点连接 [name]","formatEN":"test cluster node connection [name]"}
func (b *BaseApi) TestClusterNode(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	info, err := clusterNodeService.TestConnection(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, info)
}

// @Tags Cluster Node
// @Summary Check all cluster nodes
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes/check/all [post]
// @x-panel-log {"bodyKeys":[],"paramKeys":[],"BeforeFunctions":[],"formatZH":"重新检查全部集群节点","formatEN":"recheck all cluster nodes"}
func (b *BaseApi) CheckAllClusterNodes(c *gin.Context) {
	if err := clusterNodeService.CheckAll(); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster Node
// @Summary Get cluster nodes overview
// @Success 200 {object} dto.ClusterNodeOverview
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes/overview [get]
func (b *BaseApi) GetClusterNodeOverview(c *gin.Context) {
	overview, err := clusterNodeService.Overview()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, overview)
}

// @Tags Cluster Node
// @Summary Refresh and get the aggregate cluster overview (per-node CPU/memory + app/website totals)
// @Success 200 {object} dto.ClusterAggregateOverview
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes/overview/aggregate [get]
// @x-panel-log {"bodyKeys":[],"paramKeys":[],"BeforeFunctions":[],"formatZH":"刷新集群聚合概览","formatEN":"refresh cluster aggregate overview"}
func (b *BaseApi) GetClusterAggregateOverview(c *gin.Context) {
	overview, err := clusterNodeService.Aggregate()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, overview)
}

// @Tags Cluster Node
// @Summary List cluster node switcher options
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/nodes/options [get]
// The bottom node switcher reads this (local only): the master entry is
// added by the frontend, the rest are the registered panel nodes.
func (b *BaseApi) ListClusterNodeOptions(c *gin.Context) {
	options, err := clusterNodeService.ListOptions()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, options)
}

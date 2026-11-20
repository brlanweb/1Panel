package v2

import (
	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags Cluster DB Replication
// @Summary List database replication topologies
// @Success 200 {object} []dto.ClusterDbReplicationInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/search [post]
func (b *BaseApi) SearchClusterDbReplications(c *gin.Context) {
	list, err := clusterDbReplicationService.List()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

// @Tags Cluster DB Replication
// @Summary Get one database replication topology
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Success 200 {object} dto.ClusterDbReplicationInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/get [post]
func (b *BaseApi) GetClusterDbReplication(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	info, err := clusterDbReplicationService.Get(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, info)
}

// @Tags Cluster DB Replication
// @Summary Create a database replication topology
// @Accept json
// @Param request body dto.ClusterDbReplicationCreate true "request"
// @Success 200 {object} map[string]uint
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications [post]
// @x-panel-log { "bodyKeys":["name"], "paramKeys":[], "BeforeFunctions":[], "formatZH":"添加数据库同步拓扑 [name]", "formatEN":"add database replication topology [name]" }
func (b *BaseApi) CreateClusterDbReplication(c *gin.Context) {
	var req dto.ClusterDbReplicationCreate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	id, err := clusterDbReplicationService.Create(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, map[string]uint{"id": id})
}

// @Tags Cluster DB Replication
// @Summary Update a database replication topology (name / replication credentials)
// @Accept json
// @Param request body dto.ClusterDbReplicationUpdate true "request"
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/update [post]
func (b *BaseApi) UpdateClusterDbReplication(c *gin.Context) {
	var req dto.ClusterDbReplicationUpdate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterDbReplicationService.Update(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster DB Replication
// @Summary Delete a database replication topology
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/del [post]
func (b *BaseApi) DeleteClusterDbReplication(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterDbReplicationService.Delete(req.ID); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster DB Replication
// @Summary List database instances on a node (for the wizard)
// @Accept json
// @Param request body dto.ClusterDbInstancesReq true "request"
// @Success 200 {object} []dto.ClusterDbInstance
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/instances/search [post]
func (b *BaseApi) SearchClusterDbInstances(c *gin.Context) {
	var req dto.ClusterDbInstancesReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := clusterDbReplicationService.ListInstances(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

// @Tags Cluster DB Replication
// @Summary List schemas inside one instance on a node (for the wizard)
// @Accept json
// @Param request body dto.ClusterDbSchemasReq true "request"
// @Success 200 {object} []dto.ClusterDbSchema
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/schemas/search [post]
func (b *BaseApi) SearchClusterDbSchemas(c *gin.Context) {
	var req dto.ClusterDbSchemasReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := clusterDbReplicationService.ListSchemas(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

// @Tags Cluster DB Replication
// @Summary Run preflight checks for a topology
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Success 200 {object} []dto.ClusterDbPreflightCheck
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/preflight [post]
func (b *BaseApi) PreflightClusterDbReplication(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	checks, err := clusterDbReplicationService.Preflight(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, checks)
}

// @Tags Cluster DB Replication
// @Summary List the manual steps of a topology
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Success 200 {object} []dto.ClusterDbManualStep
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/manual-steps [post]
func (b *BaseApi) ListClusterDbManualSteps(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	steps, err := clusterDbReplicationService.ManualSteps(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, steps)
}

// @Tags Cluster DB Replication
// @Summary Mark manual steps done
// @Accept json
// @Param request body dto.ClusterDbConfirmReq true "request"
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/manual-steps/done [post]
func (b *BaseApi) MarkClusterDbManualStepsDone(c *gin.Context) {
	var req dto.ClusterDbConfirmReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterDbReplicationService.MarkManualStepsDone(req.ID, req.ManualStepKeys); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster DB Replication
// @Summary Write replication config and restart the instances (master config step)
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/configure [post]
func (b *BaseApi) ConfigureClusterDbReplication(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterDbReplicationService.ConfigureMaster(req.ID); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster DB Replication
// @Summary Start the initial snapshot in the background
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/snapshot/start [post]
func (b *BaseApi) StartClusterDbSnapshot(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterDbReplicationService.StartSnapshot(req.ID); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster DB Replication
// @Summary Confirm the manual replication-start SQL was executed
// @Accept json
// @Param request body dto.ClusterDbConfirmReq true "request"
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/confirm-started [post]
func (b *BaseApi) ConfirmClusterDbReplicationStarted(c *gin.Context) {
	var req dto.ClusterDbConfirmReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterDbReplicationService.ConfirmReplicationStarted(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster DB Replication
// @Summary Refresh replication status (master binlog position, reachability)
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Success 200 {object} dto.ClusterDbReplicationInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/refresh [post]
func (b *BaseApi) RefreshClusterDbReplication(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	info, err := clusterDbReplicationService.RefreshStatus(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, info)
}

// @Tags Cluster DB Replication
// @Summary List the manual failover steps for a topology
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Success 200 {object} []dto.ClusterDbManualStep
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/promote/steps [post]
func (b *BaseApi) ListClusterDbPromoteSteps(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	steps, err := clusterDbReplicationService.PromoteSteps(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, steps)
}

// @Tags Cluster DB Replication
// @Summary Manually promote the replica to master
// @Accept json
// @Param request body dto.ClusterDbPromoteReq true "request"
// @Success 200 {object} dto.ClusterDbFailoverInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/promote [post]
// @x-panel-log { "bodyKeys":["id"], "paramKeys":[], "BeforeFunctions":[], "formatZH":"数据库故障转移提升从库 [id]", "formatEN":"database failover promote replica [id]" }
func (b *BaseApi) PromoteClusterDbReplication(c *gin.Context) {
	var req dto.ClusterDbPromoteReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	info, err := clusterDbReplicationService.Promote(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, info)
}

// @Tags Cluster DB Replication
// @Summary List failover history of a topology
// @Accept json
// @Param request body dto.ClusterDbIDReq true "request"
// @Success 200 {object} []dto.ClusterDbFailoverInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/db-replications/failovers [post]
func (b *BaseApi) ListClusterDbFailovers(c *gin.Context) {
	var req dto.ClusterDbIDReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := clusterDbReplicationService.ListFailovers(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

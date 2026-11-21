package v2

import (
	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags Cluster Sync
// @Summary List resources of one kind on a source node (for sync selection)
// @Accept json
// @Param request body dto.ClusterSyncResourceSearch true "request"
// @Success 200 {object} dto.ClusterSyncResourcePage
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/sync/resources/search [post]
func (b *BaseApi) SearchClusterSyncResources(c *gin.Context) {
	var req dto.ClusterSyncResourceSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := clusterSyncService.SearchResources(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

// @Tags Cluster Sync
// @Summary Preview a sync execution (per resource x target action, no changes made)
// @Accept json
// @Param request body dto.ClusterSyncPreviewReq true "request"
// @Success 200 {object} []dto.ClusterSyncPreviewItem
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/sync/preview [post]
func (b *BaseApi) PreviewClusterSync(c *gin.Context) {
	var req dto.ClusterSyncPreviewReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	items, err := clusterSyncService.Preview(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, items)
}

// @Tags Cluster Sync
// @Summary Execute a sync task (creates a task; conflicts are skipped unless overwriteConflicts is true)
// @Accept json
// @Param request body dto.ClusterSyncExecuteReq true "request"
// @Success 200 {object} dto.ClusterSyncTaskInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/sync/execute [post]
// @x-panel-log {"bodyKeys":["kind"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"执行集群资源同步 [kind]","formatEN":"execute cluster resource sync [kind]"}
func (b *BaseApi) ExecuteClusterSync(c *gin.Context) {
	var req dto.ClusterSyncExecuteReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	taskID, err := clusterSyncService.Execute(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, gin.H{"taskId": taskID})
}

// @Tags Cluster Sync
// @Summary Search sync tasks
// @Accept json
// @Param request body dto.ClusterSyncTaskSearch true "request"
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/sync/tasks/search [post]
func (b *BaseApi) SearchClusterSyncTasks(c *gin.Context) {
	var req dto.ClusterSyncTaskSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	total, list, err := clusterSyncService.SearchTasks(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, dto.PageResult{
		Total: total,
		Items: list,
	})
}

// @Tags Cluster Sync
// @Summary List per-item records of one sync task
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200 {object} []dto.ClusterSyncRecordInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/sync/tasks/records [post]
func (b *BaseApi) ListClusterSyncTaskRecords(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := clusterSyncService.TaskRecords(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

// @Tags Cluster Sync
// @Summary Cancel a running sync task and remove its temporary files
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/sync/tasks/cancel [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"取消资源同步任务 [id]","formatEN":"cancel resource sync task [id]"}
func (b *BaseApi) CancelClusterSyncTask(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterSyncService.CancelTask(req.ID); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Cluster Sync
// @Summary List database services available on every given node (app transfer)
// @Accept json
// @Param request body dto.ClusterSyncAppDBReq true "request"
// @Success 200 {object} []dto.ClusterSyncAppDB
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/sync/app/databases [post]
func (b *BaseApi) ListClusterSyncAppDatabases(c *gin.Context) {
	var req dto.ClusterSyncAppDBReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := clusterSyncService.AppDatabases(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

// @Tags Cluster Sync
// @Summary Delete a file or directory on a node (file transfer page)
// @Accept json
// @Param request body dto.ClusterSyncFileDelete true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /core/cluster/sync/files/del [post]
func (b *BaseApi) DeleteClusterSyncFile(c *gin.Context) {
	var req dto.ClusterSyncFileDelete
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := clusterSyncService.DeleteFile(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

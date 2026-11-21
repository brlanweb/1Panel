package router

import (
	v2 "github.com/1Panel-dev/1Panel/core/app/api/v2"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/gin-gonic/gin"
)

type ClusterSyncRouter struct {
}

func (a *ClusterSyncRouter) InitRouter(Router *gin.RouterGroup) {
	clusterSyncRouter := Router.Group("cluster/sync").
		Use(middleware.SessionAuth()).
		Use(middleware.PasswordExpired())

	baseApi := v2.ApiGroupApp.BaseApi
	{
		clusterSyncRouter.POST("/resources/search", baseApi.SearchClusterSyncResources)
		clusterSyncRouter.POST("/preview", baseApi.PreviewClusterSync)
		clusterSyncRouter.POST("/execute", baseApi.ExecuteClusterSync)
		clusterSyncRouter.POST("/tasks/search", baseApi.SearchClusterSyncTasks)
		clusterSyncRouter.POST("/tasks/records", baseApi.ListClusterSyncTaskRecords)
		clusterSyncRouter.POST("/app/databases", baseApi.ListClusterSyncAppDatabases)
		clusterSyncRouter.POST("/files/del", baseApi.DeleteClusterSyncFile)
	}
}

package router

import (
	v2 "github.com/1Panel-dev/1Panel/core/app/api/v2"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/gin-gonic/gin"
)

type ClusterDbReplicationRouter struct {
}

func (a *ClusterDbReplicationRouter) InitRouter(Router *gin.RouterGroup) {
	clusterDbReplicationRouter := Router.Group("cluster/db-replications").
		Use(middleware.SessionAuth()).
		Use(middleware.PasswordExpired())

	baseApi := v2.ApiGroupApp.BaseApi
	{
		clusterDbReplicationRouter.POST("/search", baseApi.SearchClusterDbReplications)
		clusterDbReplicationRouter.POST("/get", baseApi.GetClusterDbReplication)
		clusterDbReplicationRouter.POST("", baseApi.CreateClusterDbReplication)
		clusterDbReplicationRouter.POST("/update", baseApi.UpdateClusterDbReplication)
		clusterDbReplicationRouter.POST("/del", baseApi.DeleteClusterDbReplication)
		clusterDbReplicationRouter.POST("/instances/search", baseApi.SearchClusterDbInstances)
		clusterDbReplicationRouter.POST("/schemas/search", baseApi.SearchClusterDbSchemas)
		clusterDbReplicationRouter.POST("/preflight", baseApi.PreflightClusterDbReplication)
		clusterDbReplicationRouter.POST("/manual-steps", baseApi.ListClusterDbManualSteps)
		clusterDbReplicationRouter.POST("/manual-steps/done", baseApi.MarkClusterDbManualStepsDone)
		clusterDbReplicationRouter.POST("/configure", baseApi.ConfigureClusterDbReplication)
		clusterDbReplicationRouter.POST("/snapshot/start", baseApi.StartClusterDbSnapshot)
		clusterDbReplicationRouter.POST("/confirm-started", baseApi.ConfirmClusterDbReplicationStarted)
		clusterDbReplicationRouter.POST("/refresh", baseApi.RefreshClusterDbReplication)
		clusterDbReplicationRouter.POST("/promote/steps", baseApi.ListClusterDbPromoteSteps)
		clusterDbReplicationRouter.POST("/promote", baseApi.PromoteClusterDbReplication)
		clusterDbReplicationRouter.POST("/failovers", baseApi.ListClusterDbFailovers)
	}
}

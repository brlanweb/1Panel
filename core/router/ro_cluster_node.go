package router

import (
	v2 "github.com/1Panel-dev/1Panel/core/app/api/v2"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/gin-gonic/gin"
)

type ClusterNodeRouter struct {
}

func (a *ClusterNodeRouter) InitRouter(Router *gin.RouterGroup) {
	clusterNodeRouter := Router.Group("cluster/nodes").
		Use(middleware.SessionAuth()).
		Use(middleware.PasswordExpired())

	baseApi := v2.ApiGroupApp.BaseApi
	{
		clusterNodeRouter.POST("/search", baseApi.SearchClusterNode)
		clusterNodeRouter.POST("", baseApi.CreateClusterNode)
		clusterNodeRouter.POST("/update", baseApi.UpdateClusterNode)
		clusterNodeRouter.POST("/del", baseApi.DeleteClusterNode)
		clusterNodeRouter.POST("/test", baseApi.TestClusterNode)
		clusterNodeRouter.POST("/check/all", baseApi.CheckAllClusterNodes)
		clusterNodeRouter.GET("/overview", baseApi.GetClusterNodeOverview)
		clusterNodeRouter.GET("/overview/aggregate", baseApi.GetClusterAggregateOverview)
		clusterNodeRouter.GET("/options", baseApi.ListClusterNodeOptions)
	}
}

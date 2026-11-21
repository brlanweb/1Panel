package router

import (
	v2 "github.com/1Panel-dev/1Panel/core/app/api/v2"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/gin-gonic/gin"
)

type ClusterMonitorRouter struct{}

func (a *ClusterMonitorRouter) InitRouter(Router *gin.RouterGroup) {
	clusterMonitorRouter := Router.Group("cluster/resource").
		Use(middleware.SessionAuth()).
		Use(middleware.PasswordExpired())

	baseApi := v2.ApiGroupApp.BaseApi
	{
		clusterMonitorRouter.GET("/realtime", baseApi.LoadClusterMonitorRealtime)
		clusterMonitorRouter.POST("/static", baseApi.LoadClusterMonitorStatic)
	}
}

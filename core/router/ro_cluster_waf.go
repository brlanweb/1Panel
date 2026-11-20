package router

import (
	v2 "github.com/1Panel-dev/1Panel/core/app/api/v2"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/gin-gonic/gin"
)

type ClusterWafRouter struct {
}

func (a *ClusterWafRouter) InitRouter(Router *gin.RouterGroup) {
	clusterWafRouter := Router.Group("cluster/waf").
		Use(middleware.SessionAuth()).
		Use(middleware.PasswordExpired())

	baseApi := v2.ApiGroupApp.BaseApi
	{
		clusterWafRouter.GET("/overview", baseApi.GetClusterWafOverview)
		clusterWafRouter.POST("/websites/search", baseApi.SearchClusterWafWebsites)
	}
}

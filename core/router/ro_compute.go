package router

import (
	v2 "github.com/1Panel-dev/1Panel/core/app/api/v2"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/gin-gonic/gin"
)

type ComputeRouter struct{}

func (a *ComputeRouter) InitRouter(Router *gin.RouterGroup) {
	r := Router.Group("compute").
		Use(middleware.SessionAuth()).
		Use(middleware.PasswordExpired())

	b := v2.ApiGroupApp.BaseApi
	{
		r.GET("/overview", b.ComputeOverview)
		r.POST("/watch", b.ComputeWatch)

		r.POST("/hosts/search", b.ComputeListHosts)
		r.POST("/hosts/candidates", b.ComputeHostCandidates)
		r.POST("/hosts/import", b.ComputeImportHosts)
		r.POST("/hosts/update", b.ComputeUpdateHost)
		r.POST("/hosts/del", b.ComputeDeleteHost)
		r.POST("/hosts/schedulable", b.ComputeSetSchedulable)
		r.POST("/hosts/refresh", b.ComputeRefreshHost)

		r.GET("/topology", b.ComputeTopology)
		r.POST("/topology/link", b.ComputeCreateLink)
		r.POST("/topology/link/del", b.ComputeDeleteLink)

		r.GET("/runtimes", b.ComputeListRuntimes)
		r.POST("/runtimes", b.ComputeSaveRuntime)
		r.POST("/runtimes/del", b.ComputeDeleteRuntime)
		r.GET("/profiles", b.ComputeListProfiles)
		r.POST("/profiles", b.ComputeSaveProfile)
		r.POST("/profiles/del", b.ComputeDeleteProfile)

		r.GET("/instances", b.ComputeListInstances)
		r.POST("/instances", b.ComputeSaveInstance)
		r.POST("/instances/adopt", b.ComputeAdopt)
		r.POST("/instances/route", b.ComputeSetRoute)
		r.POST("/instances/operate", b.ComputeOperateInstance)

		r.POST("/deploy", b.ComputeDeploy)
		r.POST("/switch", b.ComputeSwitch)
		r.POST("/jobs/search", b.ComputePageJobs)
		r.POST("/jobs/get", b.ComputeGetJob)

		r.GET("/consumers", b.ComputeListConsumers)
		r.POST("/consumers", b.ComputeSaveConsumer)
		r.POST("/consumers/reset", b.ComputeResetConsumer)
		r.POST("/consumers/del", b.ComputeDeleteConsumer)
		r.GET("/setting", b.ComputeGetSetting)
		r.POST("/setting", b.ComputeUpdateSetting)

		r.GET("/policies", b.ComputeListPolicies)
		r.POST("/policies", b.ComputeSavePolicy)
		r.POST("/policies/del", b.ComputeDeletePolicy)
		r.GET("/suggestions", b.ComputeSuggestions)
		r.POST("/suggestions/operate", b.ComputeOperateSuggestion)
	}
}

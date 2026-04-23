package apiserver

import (
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/gin-gonic/gin"
)

// registerStatisticsProtectedRoutes 注册 Statistics 模块相关的受保护路由。
func (r *Router) registerStatisticsProtectedRoutes(apiV1 *gin.RouterGroup) {
	statisticsModule := r.container.StatisticsModule
	if statisticsModule == nil || statisticsModule.Handler == nil {
		return
	}

	statistics := apiV1.Group("/statistics")
	{
		adminStatistics := statistics.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		adminStatistics.GET("/overview", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetOverview,
		)...)
		adminStatistics.GET("/clinicians", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.ListClinicianStatistics,
		)...)
		adminStatistics.GET("/clinicians/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetClinicianStatistics,
		)...)
		adminStatistics.GET("/entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.ListAssessmentEntryStatistics,
		)...)
		adminStatistics.GET("/entries/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetAssessmentEntryStatistics,
		)...)
		adminStatistics.GET("/system", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetSystemStatistics,
		)...)
		adminStatistics.GET("/questionnaires/:code", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetQuestionnaireStatistics,
		)...)
		statistics.GET("/testees/:testee_id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetTesteeStatistics,
		)...)
		statistics.GET("/testees/:testee_id/periodic", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetTesteePeriodicStatistics,
		)...)
		adminStatistics.GET("/plans/:plan_id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetPlanStatistics,
		)...)
		clinicianStatistics := statistics.Group("/clinicians/me")
		clinicianStatistics.GET("/overview", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetCurrentClinicianOverview,
		)...)
		clinicianStatistics.GET("/entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.ListCurrentClinicianEntryStatistics,
		)...)
		clinicianStatistics.GET("/testees-summary", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsModule.Handler.GetCurrentClinicianTesteeSummary,
		)...)
		contentStatistics := statistics.Group("", restmiddleware.RequireAnyCapabilityMiddleware(
			restmiddleware.CapabilityManageQuestionnaires,
			restmiddleware.CapabilityManageScales,
		))
		contentStatistics.POST("/questionnaires/batch", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			statisticsModule.Handler.BatchQuestionnaireStatistics,
		)...)
	}
}

func (r *Router) registerStatisticsInternalRoutes(internalV1 *gin.RouterGroup) {
	statisticsModule := r.container.StatisticsModule
	if statisticsModule == nil || statisticsModule.Handler == nil {
		return
	}

	statistics := internalV1.Group("/statistics", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	sync := statistics.Group("/sync")
	sync.POST("/daily", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.SyncDailyStatistics,
	)...)
	sync.POST("/accumulated", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.SyncAccumulatedStatistics,
	)...)
	sync.POST("/plan", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.SyncPlanStatistics,
	)...)
}

func (r *Router) registerCacheGovernanceInternalRoutes(internalV1 *gin.RouterGroup) {
	statisticsModule := r.container.StatisticsModule
	if statisticsModule == nil || statisticsModule.Handler == nil {
		return
	}

	governance := internalV1.Group("/cache/governance", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	governance.POST("/repair-complete", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.RepairComplete,
	)...)
	governance.POST("/warmup-targets", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsModule.Handler.WarmupTargets,
	)...)
	governance.GET("/status", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		statisticsModule.Handler.CacheGovernanceStatus,
	)...)
	governance.GET("/hotset", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		statisticsModule.Handler.CacheGovernanceHotset,
	)...)
}

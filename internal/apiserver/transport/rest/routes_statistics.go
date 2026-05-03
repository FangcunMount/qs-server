package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) newStatisticsHandler() *handler.StatisticsHandler {
	if !r.deps.Statistics.Enabled {
		return nil
	}
	statisticsHandler := handler.NewStatisticsHandler(
		r.deps.Statistics.SystemStatisticsService,
		r.deps.Statistics.QuestionnaireStatisticsService,
		r.deps.Statistics.TesteeStatisticsService,
		r.deps.Statistics.PlanStatisticsService,
		r.deps.Statistics.ReadService,
		r.deps.Statistics.PeriodicStatsService,
		r.deps.Statistics.SyncService,
	)
	if r.deps.Statistics.TesteeAccessService != nil {
		statisticsHandler.SetTesteeAccessService(r.deps.Statistics.TesteeAccessService)
	}
	if r.deps.Statistics.WarmupCoordinator != nil {
		statisticsHandler.SetWarmupCoordinator(r.deps.Statistics.WarmupCoordinator)
	}
	if r.deps.Statistics.CacheGovernanceStatusService != nil {
		statisticsHandler.SetCacheGovernanceStatusService(r.deps.Statistics.CacheGovernanceStatusService)
	}
	return statisticsHandler
}

// registerStatisticsProtectedRoutes 注册 Statistics 模块相关的受保护路由。
func (r *Router) registerStatisticsProtectedRoutes(apiV1 *gin.RouterGroup) {
	statisticsHandler := r.newStatisticsHandler()
	if statisticsHandler == nil {
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
			statisticsHandler.GetOverview,
		)...)
		adminStatistics.GET("/clinicians", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.ListClinicianStatistics,
		)...)
		adminStatistics.GET("/clinicians/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetClinicianStatistics,
		)...)
		adminStatistics.GET("/entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.ListAssessmentEntryStatistics,
		)...)
		adminStatistics.GET("/entries/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetAssessmentEntryStatistics,
		)...)
		adminStatistics.GET("/system", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetSystemStatistics,
		)...)
		adminStatistics.GET("/questionnaires/:code", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetQuestionnaireStatistics,
		)...)
		statistics.GET("/testees/:testee_id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetTesteeStatistics,
		)...)
		statistics.GET("/testees/:testee_id/periodic", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetTesteePeriodicStatistics,
		)...)
		adminStatistics.GET("/plans/:plan_id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetPlanStatistics,
		)...)
		clinicianStatistics := statistics.Group("/clinicians/me")
		clinicianStatistics.GET("/overview", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetCurrentClinicianOverview,
		)...)
		clinicianStatistics.GET("/entries", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.ListCurrentClinicianEntryStatistics,
		)...)
		clinicianStatistics.GET("/testees-summary", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			statisticsHandler.GetCurrentClinicianTesteeSummary,
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
			statisticsHandler.BatchQuestionnaireStatistics,
		)...)
	}
}

func (r *Router) registerStatisticsInternalRoutes(internalV1 *gin.RouterGroup) {
	statisticsHandler := r.newStatisticsHandler()
	if statisticsHandler == nil {
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
		statisticsHandler.SyncDailyStatistics,
	)...)
	sync.POST("/org-snapshot", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsHandler.SyncOrgSnapshotStatistics,
	)...)
	sync.POST("/plan", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsHandler.SyncPlanStatistics,
	)...)
}

func (r *Router) registerCacheGovernanceInternalRoutes(internalV1 *gin.RouterGroup) {
	statisticsHandler := r.newStatisticsHandler()
	if statisticsHandler == nil {
		return
	}

	governance := internalV1.Group("/cache/governance", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	governance.POST("/repair-complete", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsHandler.RepairComplete,
	)...)
	governance.POST("/warmup-targets", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		statisticsHandler.WarmupTargets,
	)...)
	governance.GET("/status", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		statisticsHandler.CacheGovernanceStatus,
	)...)
	governance.GET("/hotset", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		statisticsHandler.CacheGovernanceHotset,
	)...)
}

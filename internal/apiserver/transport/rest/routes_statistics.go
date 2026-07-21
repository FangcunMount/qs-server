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

func (r *Router) newStatisticsV2Handler() *handler.StatisticsV2Handler {
	if !r.deps.Statistics.Enabled || r.deps.Statistics.V2ReadService == nil || r.deps.Statistics.V2Coordinator == nil || r.deps.Statistics.V2RunStore == nil {
		return nil
	}
	return handler.NewStatisticsV2Handler(r.deps.Statistics.V2ReadService, r.deps.Statistics.V2Coordinator, r.deps.Statistics.V2RunStore)
}

func (r *Router) registerStatisticsV2ProtectedRoutes(apiV2 *gin.RouterGroup) {
	h := r.newStatisticsV2Handler()
	if h == nil {
		return
	}
	statistics := apiV2.Group("/statistics")
	admin := statistics.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	admin.GET("/overview", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Overview)...)
	admin.GET("/clinicians", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Clinicians)...)
	admin.GET("/clinicians/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Clinician)...)
	admin.GET("/entries", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Entries)...)
	admin.GET("/entries/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Entry)...)
	me := statistics.Group("/clinicians/me")
	me.GET("/overview", r.rateLimitedHandlers(rateLimitBudgetQuery, h.CurrentClinicianOverview)...)
	me.GET("/entries", r.rateLimitedHandlers(rateLimitBudgetQuery, h.CurrentClinicianEntries)...)
	me.GET("/testees-summary", r.rateLimitedHandlers(rateLimitBudgetQuery, h.CurrentClinicianTestees)...)
	content := statistics.Group("", restmiddleware.RequireAnyCapabilityMiddleware(restmiddleware.CapabilityManageQuestionnaires, restmiddleware.CapabilityManageAssessmentModels))
	content.POST("/contents/batch", r.rateLimitedHandlers(rateLimitBudgetSubmit, h.Contents)...)
}

func (r *Router) registerStatisticsV2InternalRoutes(internalV2 *gin.RouterGroup) {
	h := r.newStatisticsV2Handler()
	if h == nil {
		return
	}
	runs := internalV2.Group("/statistics/runs", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	runs.POST("", r.rateLimitedHandlers(rateLimitBudgetAdminSubmit, h.CreateRun)...)
	runs.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, h.ListRuns)...)
	runs.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, h.GetRun)...)
	runs.POST("/:id/resume-cache", r.rateLimitedHandlers(rateLimitBudgetAdminSubmit, h.ResumeCache)...)
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
		adminStatistics.GET("/overview", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.GetOverview)...)
		adminStatistics.GET("/clinicians", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.ListClinicianStatistics)...)
		adminStatistics.GET("/clinicians/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.GetClinicianStatistics)...)
		adminStatistics.GET("/entries", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.ListAssessmentEntryStatistics)...)
		adminStatistics.GET("/entries/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.GetAssessmentEntryStatistics)...)
		statistics.GET("/testees/:testee_id/periodic", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.GetTesteePeriodicStatistics)...)
		clinicianStatistics := statistics.Group("/clinicians/me")
		clinicianStatistics.GET("/overview", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.GetCurrentClinicianOverview)...)
		clinicianStatistics.GET("/entries", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.ListCurrentClinicianEntryStatistics)...)
		clinicianStatistics.GET("/testees-summary", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.GetCurrentClinicianTesteeSummary)...)
		contentStatistics := statistics.Group("", restmiddleware.RequireAnyCapabilityMiddleware(
			restmiddleware.CapabilityManageQuestionnaires,
			restmiddleware.CapabilityManageAssessmentModels,
		))
		contentStatistics.POST("/contents/batch", r.rateLimitedHandlers(rateLimitBudgetSubmit, statisticsHandler.BatchContentStatistics)...)
	}
}

func (r *Router) registerStatisticsInternalRoutes(internalV1 *gin.RouterGroup) {
	statisticsHandler := r.newStatisticsHandler()
	if statisticsHandler == nil {
		return
	}

	statistics := internalV1.Group("/statistics", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	sync := statistics.Group("/sync")
	sync.POST("/daily", r.rateLimitedHandlers(rateLimitBudgetSubmit, statisticsHandler.SyncDailyStatistics)...)
	sync.POST("/org-snapshot", r.rateLimitedHandlers(rateLimitBudgetSubmit, statisticsHandler.SyncOrgSnapshotStatistics)...)
	sync.POST("/plan", r.rateLimitedHandlers(rateLimitBudgetSubmit, statisticsHandler.SyncPlanStatistics)...)
}

func (r *Router) registerCacheGovernanceInternalRoutes(internalV1 *gin.RouterGroup) {
	statisticsHandler := r.newStatisticsHandler()
	if statisticsHandler == nil {
		return
	}

	governance := internalV1.Group("/cache/governance", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	governance.POST("/repair-complete", r.rateLimitedHandlers(rateLimitBudgetSubmit, statisticsHandler.RepairComplete)...)
	governance.POST("/warmup-targets", r.rateLimitedHandlers(rateLimitBudgetSubmit, statisticsHandler.WarmupTargets)...)
	governance.GET("/status", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.CacheGovernanceStatus)...)
	governance.GET("/hotset", r.rateLimitedHandlers(rateLimitBudgetQuery, statisticsHandler.CacheGovernanceHotset)...)
}

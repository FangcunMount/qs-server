package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) newStatisticsHandler() *handler.StatisticsHandler {
	if !r.deps.Statistics.Enabled || r.deps.Statistics.ReadService == nil || r.deps.Statistics.Coordinator == nil || r.deps.Statistics.RunStore == nil {
		return nil
	}
	return handler.NewStatisticsHandler(r.deps.Statistics.ReadService, r.deps.Statistics.Coordinator, r.deps.Statistics.RunStore)
}

func (r *Router) registerStatisticsProtectedRoutes(apiV2 *gin.RouterGroup) {
	h := r.newStatisticsHandler()
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

func (r *Router) registerStatisticsInternalRoutes(internalV2 *gin.RouterGroup) {
	h := r.newStatisticsHandler()
	if h == nil {
		return
	}
	runs := internalV2.Group("/statistics/runs", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	runs.POST("", r.rateLimitedHandlers(rateLimitBudgetAdminSubmit, h.CreateRun)...)
	runs.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, h.ListRuns)...)
	runs.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, h.GetRun)...)
	runs.POST("/:id/resume-cache", r.rateLimitedHandlers(rateLimitBudgetAdminSubmit, h.ResumeCache)...)
}

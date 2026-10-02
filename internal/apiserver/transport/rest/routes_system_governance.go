package rest

import (
	handler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerSystemGovernanceInternalRoutes(internalV1 *gin.RouterGroup) {
	if r.deps.SystemGovernanceFacade == nil {
		return
	}
	governanceHandler := handler.NewSystemGovernanceHandler(r.deps.SystemGovernanceFacade)
	governance := internalV1.Group("/system-governance", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	governance.POST("/actions/reminder-resolutions", r.rateLimitedHandlers(rateLimitBudgetSubmit, governanceHandler.ResolveReminder)...)
	governance.GET("/actions/reminder-resolutions/:request_id", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.GetReminderResolution)...)
	governance.GET("/overview", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.Overview)...)
	governance.GET("/events", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.Events)...)
	governance.GET("/events/retry-candidates", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.RetryCandidates)...)
	governance.GET("/cache", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.Cache)...)
	governance.GET("/resilience", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.Resilience)...)
	governance.GET("/actions", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.Actions)...)
	governance.GET("/actions/pending-reconciliations", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.PendingReplayAudits)...)
	governance.GET("/actions/delivery-replay-reviews", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.DeliveryReplayReviews)...)
	governance.GET("/actions/reminder-reviews", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.ReminderReviews)...)
	governance.GET("/actions/gap-recoveries/summary", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.GetGapRecoverySummary)...)
	governance.POST("/actions/gap-recoveries", r.rateLimitedHandlers(rateLimitBudgetSubmit, governanceHandler.AuthorizeGapRecovery)...)
	governance.POST("/actions/gap-recoveries/resolve", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.ResolveGapRecovery)...)
	governance.POST("/actions/reminder-gap-recoveries", r.rateLimitedHandlers(rateLimitBudgetSubmit, governanceHandler.AuthorizeReminderGapRecovery)...)
	governance.POST("/actions/reminder-gap-recoveries/resolve", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.ResolveReminderGapRecovery)...)
	governance.POST("/actions/delivery-resolutions", r.rateLimitedHandlers(rateLimitBudgetSubmit, governanceHandler.ResolveDelivery)...)
	governance.GET("/actions/delivery-resolutions/:request_id", r.rateLimitedHandlers(rateLimitBudgetQuery, governanceHandler.GetDeliveryResolution)...)
	governance.POST("/actions/:action_id/runs", r.rateLimitedHandlers(rateLimitBudgetSubmit, governanceHandler.RunAction)...)
}

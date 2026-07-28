package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerHistoricalSeedInternalRoutes(internalV1 *gin.RouterGroup) {
	if r.deps.HistoricalStageReader == nil {
		return
	}
	h := handler.NewHistoricalSeedStageHandler(r.deps.HistoricalStageReader)
	g := internalV1.Group("/historical-seed", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	g.GET("/batches/:batch_id", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Batch)...)
	g.GET("/batches/:batch_id/scenarios", r.rateLimitedHandlers(rateLimitBudgetQuery, h.ScenarioQuery)...)
	g.GET("/batches/:batch_id/scenarios/:scenario_id", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Scenario)...)
}

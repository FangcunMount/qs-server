package rest

import (
	handler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerEvaluationRunInternalRoutes(internalV1 *gin.RouterGroup) {
	if r.deps.Evaluation.ProtectedQueryService == nil {
		return
	}
	runHandler := handler.NewEvaluationRunInternalHandler(r.deps.Evaluation.ProtectedQueryService)
	runs := internalV1.Group("/evaluation-runs", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	runs.GET("/failed", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		runHandler.ListRetryableFailed,
	)...)
}

package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/gin-gonic/gin"
)

// registerEvaluationV2ProtectedRoutes 注册评估模块 v2 受保护路由。
func (r *Router) registerEvaluationV2ProtectedRoutes(apiV2 *gin.RouterGroup) {
	if r.deps.Evaluation.ManagementService == nil ||
		r.deps.Evaluation.ProtectedQueryService == nil {
		return
	}
	evalHandler := handler.NewEvaluationHandler(
		r.deps.Evaluation.ManagementService,
		r.deps.Evaluation.EvaluationService,
		r.deps.Evaluation.ProtectedQueryService,
	)

	evaluations := apiV2.Group("/evaluations")
	{
		assessments := evaluations.Group("/assessments")
		{
			assessments.GET("", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.ListAssessmentsV2,
			)...)
			assessments.GET("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetAssessmentV2,
			)...)
			assessments.GET("/:id/report", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetReportV2,
			)...)
		}

		reports := evaluations.Group("/reports")
		{
			reports.GET("", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.ListReportsV2,
			)...)
		}
	}
}

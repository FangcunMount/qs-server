package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

// registerEvaluationProtectedRoutes 注册评估模块相关的受保护路由。
func (r *Router) registerEvaluationProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.Evaluation.ManagementService == nil ||
		r.deps.Evaluation.ProtectedQueryService == nil {
		return
	}
	evalHandler := handler.NewEvaluationHandler(
		r.deps.Evaluation.ManagementService,
		r.deps.Evaluation.EvaluationService,
		r.deps.Evaluation.ProtectedQueryService,
	)

	evaluations := apiV1.Group("/evaluations")
	{
		assessments := evaluations.Group("/assessments")
		{
			assessments.GET("", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.ListAssessments,
			)...)
			assessments.GET("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetAssessment,
			)...)
			assessments.GET("/:id/scores", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetScores,
			)...)
			assessments.GET("/:id/report", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetReport,
			)...)
			assessments.GET("/:id/high-risk-factors", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetHighRiskFactors,
			)...)
			assessmentAdmin := assessments.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityEvaluateAssessments))
			assessmentAdmin.POST("/:id/retry", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				evalHandler.RetryFailed,
			)...)
		}

		scores := evaluations.Group("/scores")
		{
			scores.GET("/trend", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetFactorTrend,
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
				evalHandler.ListReports,
			)...)
		}

		evaluationAdmin := evaluations.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityEvaluateAssessments))
		evaluationAdmin.POST("/batch-evaluate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			evalHandler.BatchEvaluate,
		)...)
	}
}

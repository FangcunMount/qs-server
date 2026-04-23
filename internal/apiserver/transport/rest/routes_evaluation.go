package rest

import (
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/gin-gonic/gin"
)

// registerEvaluationProtectedRoutes 注册评估模块相关的受保护路由。
func (r *Router) registerEvaluationProtectedRoutes(apiV1 *gin.RouterGroup) {
	evalHandler := r.deps.Evaluation.Handler
	if evalHandler == nil {
		return
	}

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

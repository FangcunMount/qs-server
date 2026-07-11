package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

// registerEvaluationProtectedRoutes 注册评估模块相关的受保护路由。
func (r *Router) registerEvaluationProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.Evaluation.OperatorRecoveryService == nil ||
		r.deps.Evaluation.OperatorExecutionService == nil ||
		r.deps.Evaluation.ProtectedQueryService == nil ||
		r.deps.Interpretation.ReportQueryJourney == nil ||
		r.deps.Interpretation.ReportWaitJourney == nil {
		return
	}
	evalHandler := handler.NewEvaluationHandler(
		r.deps.Evaluation.OperatorRecoveryService,
		r.deps.Evaluation.OperatorExecutionService,
		r.deps.Evaluation.ProtectedQueryService,
		r.deps.Interpretation.ReportQueryJourney,
		r.deps.Interpretation.ReportWaitJourney,
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
			assessments.GET("/:id/runs/latest", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.GetLatestAssessmentRun,
			)...)
			assessments.GET("/:id/runs", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				evalHandler.ListAssessmentRuns,
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

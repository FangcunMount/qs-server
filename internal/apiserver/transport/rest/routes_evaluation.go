package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

// registerEvaluationProtectedRoutes 注册评估模块相关的受保护路由。
func (r *Router) registerEvaluationProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.Evaluation.OperatorExecutionService == nil ||
		r.deps.Evaluation.ProtectedQueryService == nil ||
		r.deps.Interpretation.ReportQueryJourney == nil ||
		r.deps.Interpretation.ReportWaitJourney == nil {
		return
	}
	evalHandler := handler.NewEvaluationOperatorHandler(
		r.deps.Evaluation.OperatorExecutionService,
		r.deps.Evaluation.ProtectedQueryService,
		r.deps.SystemGovernanceFacade,
	)
	journeyHandler := handler.NewAssessmentReportJourneyHandler(
		r.deps.Interpretation.ReportQueryJourney,
		r.deps.Interpretation.ReportWaitJourney,
	)
	apiV1.GET("/assessments/:id/wait-report", r.rateLimitedHandlers(rateLimitBudgetWaitReport, journeyHandler.WaitReport)...)

	evaluations := apiV1.Group("/evaluations")
	{
		assessments := evaluations.Group("/assessments")
		{
			assessments.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, journeyHandler.ListAssessments)...)
			assessments.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, journeyHandler.GetAssessment)...)
			assessments.GET("/:id/scores", r.rateLimitedHandlers(rateLimitBudgetQuery, evalHandler.GetScores)...)
			assessments.GET("/:id/report", r.rateLimitedHandlers(rateLimitBudgetQuery, journeyHandler.GetReport)...)
			assessments.GET("/:id/high-risk-factors", r.rateLimitedHandlers(rateLimitBudgetQuery, evalHandler.GetHighRiskFactors)...)
			assessments.GET("/:id/runs/latest", r.rateLimitedHandlers(rateLimitBudgetQuery, evalHandler.GetLatestAssessmentRun)...)
			assessments.GET("/:id/runs", r.rateLimitedHandlers(rateLimitBudgetQuery, evalHandler.ListAssessmentRuns)...)
			assessmentAdmin := assessments.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityEvaluateAssessments))
			assessmentAdmin.POST("/:id/retry", r.rateLimitedHandlers(rateLimitBudgetSubmit, evalHandler.RetryFailed)...)
		}

		scores := evaluations.Group("/scores")
		{
			scores.GET("/trend", r.rateLimitedHandlers(rateLimitBudgetQuery, evalHandler.GetFactorTrend)...)
		}

		reports := evaluations.Group("/reports")
		{
			reports.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, journeyHandler.ListReports)...)
		}

		evaluationAdmin := evaluations.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityEvaluateAssessments))
		evaluationAdmin.POST("/batch-evaluate", r.rateLimitedHandlers(rateLimitBudgetSubmit, evalHandler.BatchEvaluate)...)
	}
}

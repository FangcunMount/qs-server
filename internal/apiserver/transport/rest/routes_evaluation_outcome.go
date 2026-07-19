package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/gin-gonic/gin"
)

// registerEvaluationOutcomeProtectedRoutes 注册评估模块 outcome 受保护路由。
func (r *Router) registerEvaluationOutcomeProtectedRoutes(apiV2 *gin.RouterGroup) {
	if r.deps.Evaluation.ProtectedQueryService == nil ||
		r.deps.Interpretation.ReportQueryJourney == nil {
		return
	}
	evalHandler := handler.NewEvaluationOperatorHandler(
		nil,
		r.deps.Evaluation.ProtectedQueryService,
	)
	journeyHandler := handler.NewAssessmentReportJourneyHandler(
		r.deps.Interpretation.ReportQueryJourney,
		nil,
	)

	evaluations := apiV2.Group("/evaluations")
	{
		assessments := evaluations.Group("/assessments")
		{
			assessments.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, evalHandler.ListAssessmentsOutcome)...)
			assessments.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, evalHandler.GetAssessmentOutcome)...)
			assessments.GET("/:id/report", r.rateLimitedHandlers(rateLimitBudgetQuery, journeyHandler.GetReportOutcome)...)
		}

		reports := evaluations.Group("/reports")
		{
			reports.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, journeyHandler.ListReportsOutcome)...)
		}
	}
}

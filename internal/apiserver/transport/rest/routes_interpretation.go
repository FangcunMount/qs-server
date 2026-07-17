package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerInterpretationProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.Interpretation.ClinicianService == nil {
		return
	}
	h := handler.NewInterpretationClinicianHandler(r.deps.Interpretation.ClinicianService)
	reports := apiV1.Group("/clinicians/me/testees/:testee_id/reports")
	reports.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, h.List)...)
	reports.GET("/:assessment_id", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Get)...)
}

func (r *Router) registerInterpretationInternalRoutes(internalV1 *gin.RouterGroup) {
	if r.deps.Interpretation.OperationsService == nil {
		return
	}
	h := handler.NewInterpretationOperationsHandler(r.deps.Interpretation.OperationsService)
	g := internalV1.Group("/interpretation", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
	g.GET("/reports/:report_id", h.FindReport)
	g.GET("/outcomes/:outcome_id/generations", h.FindOutcomeGenerations)
	g.GET("/assessments/:assessment_id/lifecycle", h.FindAssessmentLifecycle)
	g.GET("/assessments/:assessment_id/reports", h.ListAssessmentReports)
}

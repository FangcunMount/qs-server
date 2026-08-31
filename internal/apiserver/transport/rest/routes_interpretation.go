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
	g := internalV1.Group("/interpretation", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
	if r.deps.Interpretation.OperationsService != nil {
		h := handler.NewInterpretationOperationsHandler(r.deps.Interpretation.OperationsService)
		g.GET("/reports/:report_id", h.FindReport)
		g.GET("/outcomes/:outcome_id/generations", h.FindOutcomeGenerations)
		g.GET("/outcomes/:outcome_id/admission-failures", h.FindOutcomeAdmissionFailures)
		g.GET("/admission-failures", h.ListAdmissionFailures)
		g.GET("/assessments/:assessment_id/lifecycle", h.FindAssessmentLifecycle)
		g.GET("/assessments/:assessment_id/reports", h.ListAssessmentReports)
	}
	if r.deps.Interpretation.CatalogReconcile != nil {
		reconcile := handler.NewInterpretationCatalogReconcileHandler(r.deps.Interpretation.CatalogReconcile)
		g.GET("/catalog/reconcile", reconcile.Reconcile)
		g.GET("/catalog/drifts", reconcile.ListDrifts)
		g.POST("/catalog/repair-plans", reconcile.CreateRepairPlan)
	}
	if r.deps.Interpretation.ReportTemplates != nil {
		templates := handler.NewInterpretationReportTemplateHandler(r.deps.Interpretation.ReportTemplates)
		g.GET("/report-templates", templates.List)
		g.GET("/report-templates/:template_id/versions/:version", templates.Get)
		g.POST("/report-templates", templates.CreateDraft)
	}
	if r.deps.Interpretation.AIExplanationAdministration != nil {
		aiHandler := handler.NewAIExplanationAdministrationHandler(r.deps.Interpretation.AIExplanationAdministration)
		ai := g.Group("/ai-explanation")
		ai.GET("/prompt-evaluations", aiHandler.ListEvaluations)
		ai.GET("/prompt-evaluations/:run_id", aiHandler.FindEvaluation)
		ai.GET("/prompt-evaluations/:run_id/attempts/:case_id/:attempt", aiHandler.FindAttempt)
		ai.GET("/prompt-evaluations/:run_id/attempts/:case_id/:attempt/rechecks", aiHandler.ListAttemptRechecks)
		ai.GET("/prompt-evaluations/:run_id/attempts/:case_id/:attempt/rechecks/:recheck_id", aiHandler.FindAttemptRecheck)
		ai.GET("/profiles", aiHandler.ListProfiles)
		ai.GET("/profiles/:profile_id/versions/:version", aiHandler.FindProfile)
		governance := ai.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		governance.GET("/prompt-evaluation-capacity", aiHandler.FindEvaluationCapacity)
		governance.GET("/participant-capacity", aiHandler.FindParticipantCapacity)
		governance.POST("/generations/:generation_id/retry", aiHandler.RetryParticipantGeneration)
		governance.POST("/profiles", aiHandler.CreateProfileDraft)
		governance.POST("/profiles/:profile_id/versions/:version/publish", aiHandler.PublishProfile)
		governance.POST("/profiles/:profile_id/versions/:version/disable", aiHandler.DisableProfile)
	}
}

// registerInterpretationInternalV2Routes exposes the only writable Prompt
// evaluation runtime. The v1 group above intentionally keeps historical Run
// and Recheck queries but registers no v1 Prompt-evaluation mutation routes.
func (r *Router) registerInterpretationInternalV2Routes(internalV2 *gin.RouterGroup) {
	if r.deps.Interpretation.AIExplanationAdministration == nil {
		return
	}
	g := internalV2.Group("/interpretation", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
	ai := g.Group("/ai-explanation")
	aiHandler := handler.NewAIExplanationAdministrationHandler(r.deps.Interpretation.AIExplanationAdministration)
	ai.GET("/prompt-evaluations/:run_id", aiHandler.FindEvaluationV2)
	ai.GET("/prompt-evaluations/:run_id/executions/:execution_id/output", aiHandler.FindEvaluationV2Output)
	governance := ai.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	governance.POST("/prompt-evaluations", aiHandler.StartEvaluationV2)
	governance.POST("/legacy-prompt-evaluations/:run_id/attempts/:case_id/:attempt/rechecks", aiHandler.StartAttemptRecheck)
	governance.POST("/prompt-evaluations/:run_id/reviews", aiHandler.RecordReviewV2)
	governance.POST("/prompt-evaluations/:run_id/finalize", aiHandler.FinalizeEvaluationV2)
	governance.POST("/prompt-evaluations/:run_id/result-unknown/resolve", aiHandler.ResolveResultUnknownV2)
}

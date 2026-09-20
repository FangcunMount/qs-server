package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

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

}

// registerInterpretationInternalV2Routes proxies qs-ai management through QS authorization.
func (r *Router) registerInterpretationInternalV2Routes(internalV2 *gin.RouterGroup) {
	if r.deps.Interpretation.AIWorkflowRuntime != nil {
		runtime := handler.NewAIWorkflowRuntimeHandler(r.deps.Interpretation.AIWorkflowRuntime)
		group := internalV2.Group("/interpretation/ai-workflow/runtime", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		group.GET("/health", runtime.Health)
		group.GET("/requests/:request_id/timeline", runtime.Timeline)
		group.GET("/requests", runtime.List)
		group.GET("/requests/:request_id", runtime.Get)
	}

	if r.deps.Interpretation.AIWorkflowParticipants != nil {
		participants := handler.NewAIWorkflowParticipantHandler(r.deps.Interpretation.AIWorkflowParticipants)
		internalV2.GET("/interpretation/ai-workflow/participant-capacity", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin), participants.Capacity)
		group := internalV2.Group("/interpretation/ai-workflow/participants", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		group.GET("/retry-commands/:command_id", participants.RetryReceipt)
		group.GET("/:session_id", participants.Get)
		group.POST("/:session_id/retry", participants.Retry)
	}

	if r.deps.Interpretation.AIWorkflowManagement != nil {
		management := handler.NewAIWorkflowManagementHandler(r.deps.Interpretation.AIWorkflowManagement)
		internalV2.GET("/interpretation/ai-workflow/evaluation-capacity", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin), management.Capacity)
		group := internalV2.Group("/interpretation/ai-workflow/evaluations", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read := internalV2.Group("/interpretation/ai-workflow/evaluations", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		read.POST("/prepare", management.Prepare)
		read.GET("", management.List)
		read.GET("/:run_id", management.Get)
		read.GET("/:run_id/candidates", management.ListCandidates)
		read.GET("/:run_id/result-unknown", management.ListUnknowns)
		read.GET("/:run_id/gates", management.PreviewGates)
		read.GET("/:run_id/candidates/:candidate_id", management.GetCandidate)
		read.GET("/:run_id/executions", management.ListExecutions)
		read.GET("/:run_id/executions/:execution_id/output", management.GetExecutionOutput)
		group.POST("/:run_id/start", management.Start)
		group.POST("/:run_id/create", management.Create)
		group.POST("/:run_id/reviews", management.Review)
		group.POST("/:run_id/finalize", management.Finalize)
		group.POST("/:run_id/reopen-review", management.ReopenReview)
		group.POST("/:run_id/cancel", management.Cancel)
		group.POST("/:run_id/result-unknown/resolve", management.ResolveUnknown)
	}

	if r.deps.Interpretation.AIWorkflowPublications != nil {
		publications := handler.NewAIWorkflowPublicationHandler(r.deps.Interpretation.AIWorkflowPublications)
		write := internalV2.Group("/interpretation/ai-workflow/publications", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read := internalV2.Group("/interpretation/ai-workflow/publications", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		write.POST("/publish", publications.Publish)
		write.POST("/rollback", publications.Rollback)
		write.POST("/disable", publications.Disable)
		read.GET("", publications.Get)
		read.GET("/commands/:command_id", publications.GetReceipt)
		read.GET("/history", publications.ListHistory)
		read.GET("/history/:version", publications.GetHistory)
	}

	if r.deps.Interpretation.AIWorkflowSemanticDrafts != nil {
		drafts := handler.NewAIWorkflowSemanticDraftHandler(r.deps.Interpretation.AIWorkflowSemanticDrafts)
		read := internalV2.Group("/interpretation/ai-workflow/semantic-prompts/drafts", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		write := internalV2.Group("/interpretation/ai-workflow/semantic-prompts/drafts", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read.GET("/commands/:command_id", drafts.Read("receipt"))
		read.GET("/:draft_id", drafts.Read("get"))
		read.POST("/:draft_id/validate", drafts.Read("validate"))
		write.POST("/:draft_id/create", drafts.Write("create"))
		write.POST("/:draft_id/revise", drafts.Write("revise"))
		write.POST("/:draft_id/freeze", drafts.Write("freeze"))
	}
	if r.deps.Interpretation.AIWorkflowQuotas != nil {
		quotas := handler.NewAIWorkflowQuotaHandler(r.deps.Interpretation.AIWorkflowQuotas)
		read := internalV2.Group("/interpretation/ai-workflow/quotas", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		write := internalV2.Group("/interpretation/ai-workflow/quotas", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read.GET("", quotas.Read("get"))
		read.GET("/status", quotas.Read("status"))
		read.GET("/history", quotas.Read("history"))
		read.GET("/commands/:command_id", quotas.Read("receipt"))
		write.POST("/update", quotas.Write("update"))
		write.POST("/rollback", quotas.Write("rollback"))
	}
	if r.deps.Interpretation.AIWorkflowSolutions != nil {
		solutions := handler.NewAIWorkflowSolutionHandler(r.deps.Interpretation.AIWorkflowSolutions)
		read := internalV2.Group("/interpretation/ai-workflow/solutions", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		write := internalV2.Group("/interpretation/ai-workflow/solutions", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read.GET("", solutions.Read("list"))
		read.GET("/models", solutions.Read("models"))
		read.GET("/commands/:command_id", solutions.Read("receipt"))
		read.GET("/:solution_id", solutions.Read("get"))
		write.POST("/:solution_id/create", solutions.Write("create"))
		write.POST("/:solution_id/save", solutions.Write("save"))
		write.POST("/:solution_id/prepare", solutions.Write("prepare"))
	}

	if r.deps.Interpretation.AIWorkflowAssets != nil {
		catalog := handler.NewAIWorkflowCatalogHandler(r.deps.Interpretation.AIWorkflowAssets)
		read := internalV2.Group("/interpretation/ai-workflow/assets", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		read.GET("/:kind", catalog.List)
		read.GET("/:kind/detail", catalog.Get)
		read.GET("/:kind/references", catalog.References)
	}
	if r.deps.Interpretation.AIWorkflowSuites != nil {
		suites := handler.NewAIWorkflowSuiteHandler(r.deps.Interpretation.AIWorkflowSuites)
		write := internalV2.Group("/interpretation/ai-workflow/suites", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read := internalV2.Group("/interpretation/ai-workflow/suites", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		write.POST("/register", suites.Register)
		read.GET("/commands/:command_id", suites.GetReceipt)
	}

	if r.deps.Interpretation.AIWorkflowProfiles != nil {
		profiles := handler.NewAIWorkflowProfileHandler(r.deps.Interpretation.AIWorkflowProfiles)
		write := internalV2.Group("/interpretation/ai-workflow/profiles", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read := internalV2.Group("/interpretation/ai-workflow/profiles", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		write.POST("/register", profiles.Register)
		read.GET("/commands/:command_id", profiles.GetReceipt)
		read.GET("", profiles.ListLifecycle)
		read.GET("/lifecycle", profiles.GetLifecycle)
	}

	if r.deps.Interpretation.AIWorkflowPromptDrafts != nil {
		drafts := handler.NewAIWorkflowPromptDraftHandler(r.deps.Interpretation.AIWorkflowPromptDrafts)
		write := internalV2.Group("/interpretation/ai-workflow/prompt-drafts", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read := internalV2.Group("/interpretation/ai-workflow/prompt-drafts", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityAuditInterpretation))
		write.POST("/:draft_id/create", drafts.Create)
		write.POST("/:draft_id/revisions", drafts.Revise)
		write.POST("/:draft_id/freeze", drafts.Freeze)
		read.GET("/:draft_id", drafts.Get)
		read.GET("/:draft_id/lifecycle", drafts.GetLifecycle)
		read.GET("/commands/:command_id", drafts.GetReceipt)
		read.GET("/freeze-commands/:command_id", drafts.GetFreezeReceipt)
	}

}

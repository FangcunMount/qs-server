package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type actorHandlers struct {
	testee            *handler.TesteeHandler
	operatorClinician *handler.OperatorClinicianHandler
	assessmentEntry   *handler.AssessmentEntryHandler
	workbench         *handler.ClinicianWorkbenchHandler
}

func (r *Router) actorHandlers() actorHandlers {
	deps := r.deps.Actor
	handlers := actorHandlers{}
	if deps.TesteeQueryService != nil || deps.TesteeManagementService != nil || deps.TesteeBackendQueryService != nil || deps.TesteeAccessService != nil {
		handlers.testee = handler.NewTesteeHandler(
			deps.TesteeManagementService,
			deps.TesteeQueryService,
			deps.TesteeBackendQueryService,
			deps.ClinicianQueryService,
			deps.ClinicianRelationshipService,
			deps.TesteeAccessService,
			deps.TesteeScaleAnalysisService,
		)
	}
	if deps.OperatorQueryService != nil || deps.ClinicianQueryService != nil || deps.ClinicianRelationshipService != nil {
		handlers.operatorClinician = handler.NewOperatorClinicianHandler(
			deps.OperatorLifecycleService,
			deps.OperatorAuthorizationService,
			deps.OperatorQueryService,
			deps.ClinicianLifecycleService,
			deps.ClinicianQueryService,
			deps.ClinicianRelationshipService,
			deps.TesteeQueryService,
			deps.TesteeAccessService,
		)
	}
	if deps.AssessmentEntryService != nil {
		handlers.assessmentEntry = handler.NewAssessmentEntryHandler(
			deps.OperatorQueryService,
			deps.ClinicianQueryService,
			deps.AssessmentEntryService,
			deps.QRCodeService,
		)
	}
	if r.deps.Workbench.WorkbenchService != nil {
		handlers.workbench = handler.NewClinicianWorkbenchHandler(r.deps.Workbench.WorkbenchService)
	}
	return handlers
}

func (r *Router) registerActorPublicRoutes(publicAPI *gin.RouterGroup) {
	handlers := r.actorHandlers()
	if handlers.assessmentEntry == nil {
		return
	}

	publicAPI.GET("/assessment-entries/:token", handlers.assessmentEntry.ResolveAssessmentEntry)
	publicAPI.POST("/assessment-entries/:token/intake", handlers.assessmentEntry.IntakeAssessmentEntry)
}

// registerActorProtectedRoutes 注册 Actor 模块相关的受保护路由。
func (r *Router) registerActorProtectedRoutes(apiV1 *gin.RouterGroup) {
	handlers := r.actorHandlers()
	testeeHandler := handlers.testee
	operatorClinicianHandler := handlers.operatorClinician
	assessmentEntryHandler := handlers.assessmentEntry
	workbenchHandler := handlers.workbench
	if testeeHandler == nil && operatorClinicianHandler == nil && assessmentEntryHandler == nil && workbenchHandler == nil {
		return
	}

	testees := apiV1.Group("/testees")
	{
		if testeeHandler != nil {
			testees.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, testeeHandler.ListTestees)...)
			testees.GET("/by-profile-id", r.rateLimitedHandlers(rateLimitBudgetQuery, testeeHandler.GetTesteeByProfileID)...)
			testees.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, testeeHandler.GetTestee)...)
			testees.PUT("/:id", r.rateLimitedHandlers(rateLimitBudgetSubmit, testeeHandler.UpdateTestee)...)
			testees.GET("/:id/scale-analysis", r.rateLimitedHandlers(rateLimitBudgetQuery, testeeHandler.GetScaleAnalysis)...)
		}

		if operatorClinicianHandler != nil {
			testees.GET("/:id/clinicians", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.GetTesteeClinicians)...)
			testees.GET("/:id/clinician-relations", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListTesteeClinicianRelations)...)
		}
	}

	if operatorClinicianHandler != nil {
		staff := apiV1.Group("/staff", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		{
			staff.POST("", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.CreateStaff)...)
			staff.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListStaff)...)
			staff.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.GetStaff)...)
			staff.PUT("/:id", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.UpdateStaff)...)
			staff.DELETE("/:id", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.DeleteStaff)...)
		}
	}

	if workbenchHandler != nil {
		adminWorkbench := apiV1.Group("/workbench", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		adminWorkbench.GET("/queues/summary", r.rateLimitedHandlers(rateLimitBudgetQuery, workbenchHandler.GetOrgWorkbenchQueueSummary)...)
		adminWorkbench.GET("/queues/:queue_type", r.rateLimitedHandlers(rateLimitBudgetQuery, workbenchHandler.ListOrgWorkbenchQueue)...)
	}

	registerClinicianRoutes := func(group *gin.RouterGroup) {
		if operatorClinicianHandler == nil {
			return
		}
		adminClinicians := group.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		adminClinicians.POST("", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.CreateClinician)...)
		adminClinicians.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListClinicians)...)
		adminClinicians.PUT("/:id", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.UpdateClinician)...)
		adminClinicians.POST("/:id/activate", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.ActivateClinician)...)
		adminClinicians.POST("/:id/deactivate", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.DeactivateClinician)...)
		adminClinicians.POST("/:id/bind-operator", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.BindClinicianOperator)...)
		adminClinicians.POST("/:id/unbind-operator", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.UnbindClinicianOperator)...)
		me := group.Group("/me")
		me.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.GetMyClinician)...)
		me.GET("/testees", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListMyClinicianTestees)...)
		me.GET("/relations", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListMyClinicianRelations)...)
		if workbenchHandler != nil {
			me.GET("/workbench/queues/summary", r.rateLimitedHandlers(rateLimitBudgetQuery, workbenchHandler.GetMyClinicianWorkbenchQueueSummary)...)
			me.GET("/workbench/queues/:queue_type", r.rateLimitedHandlers(rateLimitBudgetQuery, workbenchHandler.ListMyClinicianWorkbenchQueue)...)
		}
		adminClinicians.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.GetClinician)...)
		adminClinicians.GET("/:id/testees", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListClinicianTestees)...)
		adminClinicians.GET("/:id/relations", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListClinicianRelations)...)
		if assessmentEntryHandler != nil {
			me.POST("/assessment-entries", r.rateLimitedHandlers(rateLimitBudgetSubmit, assessmentEntryHandler.CreateMyAssessmentEntry)...)
			me.GET("/assessment-entries", r.rateLimitedHandlers(rateLimitBudgetQuery, assessmentEntryHandler.ListMyAssessmentEntries)...)
			me.GET("/assessment-entries/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, assessmentEntryHandler.GetMyAssessmentEntry)...)
			me.POST("/assessment-entries/:id/deactivate", r.rateLimitedHandlers(rateLimitBudgetSubmit, assessmentEntryHandler.DeactivateMyAssessmentEntry)...)
			me.POST("/assessment-entries/:id/reactivate", r.rateLimitedHandlers(rateLimitBudgetSubmit, assessmentEntryHandler.ReactivateMyAssessmentEntry)...)
			adminClinicians.POST("/:id/assessment-entries", r.rateLimitedHandlers(rateLimitBudgetSubmit, assessmentEntryHandler.CreateClinicianAssessmentEntry)...)
			adminClinicians.GET("/:id/assessment-entries", r.rateLimitedHandlers(rateLimitBudgetQuery, assessmentEntryHandler.ListClinicianAssessmentEntries)...)
		}
	}

	clinicians := apiV1.Group("/clinicians")
	registerClinicianRoutes(clinicians)

	practitioners := apiV1.Group("/practitioners")
	registerClinicianRoutes(practitioners)

	if operatorClinicianHandler != nil {
		relationAdmin := apiV1.Group("/clinician-testee-relations", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		{
			relationAdmin.POST("/assign", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.AssignClinicianTestee)...)
			relationAdmin.POST("/assign-primary", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.AssignPrimaryClinicianTestee)...)
			relationAdmin.POST("/assign-attending", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.AssignAttendingClinicianTestee)...)
			relationAdmin.POST("/assign-collaborator", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.AssignCollaboratorClinicianTestee)...)
			relationAdmin.POST("/transfer-primary", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.TransferPrimaryClinicianTestee)...)
			relationAdmin.POST("/:id/unbind", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.UnbindClinicianTesteeRelation)...)
		}
	}

	if assessmentEntryHandler != nil {
		assessmentEntries := apiV1.Group("/assessment-entries", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		{
			assessmentEntries.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, assessmentEntryHandler.GetAssessmentEntry)...)
			assessmentEntries.POST("/:id/deactivate", r.rateLimitedHandlers(rateLimitBudgetSubmit, assessmentEntryHandler.DeactivateAssessmentEntry)...)
			assessmentEntries.POST("/:id/reactivate", r.rateLimitedHandlers(rateLimitBudgetSubmit, assessmentEntryHandler.ReactivateAssessmentEntry)...)
		}
	}
}

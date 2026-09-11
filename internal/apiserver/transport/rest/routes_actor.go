package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type actorHandlers struct {
	store             *handler.StoreHandler
	testee            *handler.TesteeHandler
	operatorClinician *handler.OperatorClinicianHandler
	assessmentEntry   *handler.AssessmentEntryHandler
	workbench         *handler.ClinicianWorkbenchHandler
}

func (r *Router) actorHandlers() actorHandlers {
	deps := r.deps.Actor
	handlers := actorHandlers{}
	if deps.StoreService != nil {
		handlers.store = handler.NewStoreHandler(deps.StoreService, deps.ClinicianQueryService)
	}
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

func retiredOperatorAPI(c *gin.Context) {
	c.JSON(410, gin.H{"code": 410, "message": "Operator API retired; use /api/v1/operators"})
}

func (r *Router) registerActorPublicRoutes(publicAPI *gin.RouterGroup) {
	handlers := r.actorHandlers()
	if handlers.assessmentEntry == nil {
		return
	}

	publicAPI.GET("/assessment-entries/:token", handlers.assessmentEntry.ResolveAssessmentEntry)
	publicAPI.POST("/assessment-entries/:token/intake", handlers.assessmentEntry.IntakeAssessmentEntry)
}

// Retired endpoints return only 410, without consulting identity or business data.
func (r *Router) registerRetiredActorRoutes(apiV1 *gin.RouterGroup) {
	for _, prefix := range []string{"/clinicians", "/practitioners"} {
		apiV1.Any(prefix+"/me", retiredClinicianAPI)
		apiV1.Any(prefix+"/me/*path", retiredClinicianAPI)
		apiV1.Any(prefix+"/:id/bind-operator", retiredClinicianAPI)
		apiV1.Any(prefix+"/:id/unbind-operator", retiredClinicianAPI)
	}

	apiV1.Any("/staff", retiredOperatorAPI)
	apiV1.Any("/staff/:id", retiredOperatorAPI)
}

// registerActorProtectedRoutes 注册 Actor 模块相关的受保护路由。
func (r *Router) registerActorProtectedRoutes(apiV1 *gin.RouterGroup) {
	handlers := r.actorHandlers()
	testeeHandler := handlers.testee
	operatorClinicianHandler := handlers.operatorClinician
	assessmentEntryHandler := handlers.assessmentEntry
	workbenchHandler := handlers.workbench
	if handlers.store == nil && testeeHandler == nil && operatorClinicianHandler == nil && assessmentEntryHandler == nil && workbenchHandler == nil {
		return
	}

	if h := handlers.store; h != nil {
		stores := apiV1.Group("/stores", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		stores.GET("/configuration-progress", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Progress)...)
		stores.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, h.List)...)
		stores.POST("", r.rateLimitedHandlers(rateLimitBudgetSubmit, h.Create)...)
		stores.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, h.Get)...)
		stores.PUT("/:id", r.rateLimitedHandlers(rateLimitBudgetSubmit, h.Update)...)
		stores.POST("/:id/activate", r.rateLimitedHandlers(rateLimitBudgetSubmit, h.Activate)...)
		stores.POST("/:id/deactivate", r.rateLimitedHandlers(rateLimitBudgetSubmit, h.Deactivate)...)
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
		operators := apiV1.Group("/operators", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		{
			operators.POST("", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.CreateOperator)...)
			operators.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListOperator)...)
			operators.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.GetOperator)...)
			operators.PUT("/:id", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.UpdateOperator)...)
			operators.DELETE("/:id", r.rateLimitedHandlers(rateLimitBudgetSubmit, handler.NewOperatorRetirementHandler(r.deps.Actor.OperatorRetirementService).Retire)...)
		}
		operators.GET("/:id/retirement", handler.NewOperatorRetirementHandler(r.deps.Actor.OperatorRetirementService).Status)

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
		if h := handlers.store; h != nil {
			adminClinicians.PUT("/:id/store", r.rateLimitedHandlers(rateLimitBudgetSubmit, h.Assign)...)
			adminClinicians.GET("/:id/store-history", r.rateLimitedHandlers(rateLimitBudgetQuery, h.History)...)
		}
		adminClinicians.POST("", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.CreateClinician)...)
		adminClinicians.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListClinicians)...)
		adminClinicians.PUT("/:id", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.UpdateClinician)...)
		adminClinicians.POST("/:id/activate", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.ActivateClinician)...)
		adminClinicians.POST("/:id/deactivate", r.rateLimitedHandlers(rateLimitBudgetSubmit, operatorClinicianHandler.DeactivateClinician)...)
		adminClinicians.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.GetClinician)...)
		adminClinicians.GET("/:id/testees", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListClinicianTestees)...)
		adminClinicians.GET("/:id/relations", r.rateLimitedHandlers(rateLimitBudgetQuery, operatorClinicianHandler.ListClinicianRelations)...)
		if assessmentEntryHandler != nil {
			adminClinicians.POST("/:id/assessment-entries", r.rateLimitedHandlers(rateLimitBudgetSubmit, assessmentEntryHandler.CreateClinicianAssessmentEntry)...)
			adminClinicians.GET("/:id/assessment-entries", r.rateLimitedHandlers(rateLimitBudgetQuery, assessmentEntryHandler.ListClinicianAssessmentEntries)...)
		}
	}

	clinicians := apiV1.Group("/clinicians")
	registerClinicianRoutes(clinicians)

	practitioners := apiV1.Group("/practitioners")
	practitioners.Use(observeDeprecatedPractitionerRoute)
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

func retiredClinicianAPI(c *gin.Context) {
	c.JSON(410, gin.H{"code": 410, "message": "医生后台身份与绑定入口已退役，请使用总部医生管理"})
}

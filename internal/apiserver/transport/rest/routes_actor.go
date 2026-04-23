package rest

import (
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerActorPublicRoutes(publicAPI *gin.RouterGroup) {
	if r.deps.Actor.AssessmentEntryHandler == nil {
		return
	}

	publicAPI.GET("/assessment-entries/:token", r.deps.Actor.AssessmentEntryHandler.ResolveAssessmentEntry)
	publicAPI.POST("/assessment-entries/:token/intake", r.deps.Actor.AssessmentEntryHandler.IntakeAssessmentEntry)
}

// registerActorProtectedRoutes 注册 Actor 模块相关的受保护路由。
func (r *Router) registerActorProtectedRoutes(apiV1 *gin.RouterGroup) {
	testeeHandler := r.deps.Actor.TesteeHandler
	operatorClinicianHandler := r.deps.Actor.OperatorClinicianHandler
	assessmentEntryHandler := r.deps.Actor.AssessmentEntryHandler
	if testeeHandler == nil && operatorClinicianHandler == nil && assessmentEntryHandler == nil {
		return
	}

	testees := apiV1.Group("/testees")
	{
		if testeeHandler != nil {
			testees.GET("", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				testeeHandler.ListTestees,
			)...)
			testees.GET("/by-profile-id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				testeeHandler.GetTesteeByProfileID,
			)...)
			testees.GET("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				testeeHandler.GetTestee,
			)...)
			testees.PUT("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				testeeHandler.UpdateTestee,
			)...)
			testees.GET("/:id/scale-analysis", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				testeeHandler.GetScaleAnalysis,
			)...)
		}

		if operatorClinicianHandler != nil {
			testees.GET("/:id/clinicians", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				operatorClinicianHandler.GetTesteeClinicians,
			)...)
			testees.GET("/:id/clinician-relations", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				operatorClinicianHandler.ListTesteeClinicianRelations,
			)...)
		}
	}

	if operatorClinicianHandler != nil {
		staff := apiV1.Group("/staff", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		{
			staff.POST("", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.CreateStaff,
			)...)
			staff.GET("", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				operatorClinicianHandler.ListStaff,
			)...)
			staff.GET("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				operatorClinicianHandler.GetStaff,
			)...)
			staff.PUT("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.UpdateStaff,
			)...)
			staff.DELETE("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.DeleteStaff,
			)...)
		}
	}

	registerClinicianRoutes := func(group *gin.RouterGroup) {
		if operatorClinicianHandler == nil {
			return
		}
		adminClinicians := group.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		adminClinicians.POST("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			operatorClinicianHandler.CreateClinician,
		)...)
		adminClinicians.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			operatorClinicianHandler.ListClinicians,
		)...)
		adminClinicians.PUT("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			operatorClinicianHandler.UpdateClinician,
		)...)
		adminClinicians.POST("/:id/activate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			operatorClinicianHandler.ActivateClinician,
		)...)
		adminClinicians.POST("/:id/deactivate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			operatorClinicianHandler.DeactivateClinician,
		)...)
		adminClinicians.POST("/:id/bind-operator", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			operatorClinicianHandler.BindClinicianOperator,
		)...)
		adminClinicians.POST("/:id/unbind-operator", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			operatorClinicianHandler.UnbindClinicianOperator,
		)...)
		me := group.Group("/me")
		me.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			operatorClinicianHandler.GetMyClinician,
		)...)
		me.GET("/testees", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			operatorClinicianHandler.ListMyClinicianTestees,
		)...)
		me.GET("/relations", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			operatorClinicianHandler.ListMyClinicianRelations,
		)...)
		adminClinicians.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			operatorClinicianHandler.GetClinician,
		)...)
		adminClinicians.GET("/:id/testees", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			operatorClinicianHandler.ListClinicianTestees,
		)...)
		adminClinicians.GET("/:id/relations", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			operatorClinicianHandler.ListClinicianRelations,
		)...)
		if assessmentEntryHandler := r.deps.Actor.AssessmentEntryHandler; assessmentEntryHandler != nil {
			me.POST("/assessment-entries", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				assessmentEntryHandler.CreateMyAssessmentEntry,
			)...)
			me.GET("/assessment-entries", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				assessmentEntryHandler.ListMyAssessmentEntries,
			)...)
			me.GET("/assessment-entries/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				assessmentEntryHandler.GetMyAssessmentEntry,
			)...)
			me.POST("/assessment-entries/:id/deactivate", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				assessmentEntryHandler.DeactivateMyAssessmentEntry,
			)...)
			me.POST("/assessment-entries/:id/reactivate", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				assessmentEntryHandler.ReactivateMyAssessmentEntry,
			)...)
			adminClinicians.POST("/:id/assessment-entries", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				assessmentEntryHandler.CreateClinicianAssessmentEntry,
			)...)
			adminClinicians.GET("/:id/assessment-entries", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				assessmentEntryHandler.ListClinicianAssessmentEntries,
			)...)
		}
	}

	clinicians := apiV1.Group("/clinicians")
	registerClinicianRoutes(clinicians)

	practitioners := apiV1.Group("/practitioners")
	registerClinicianRoutes(practitioners)

	if operatorClinicianHandler != nil {
		relationAdmin := apiV1.Group("/clinician-testee-relations", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		{
			relationAdmin.POST("/assign", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.AssignClinicianTestee,
			)...)
			relationAdmin.POST("/assign-primary", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.AssignPrimaryClinicianTestee,
			)...)
			relationAdmin.POST("/assign-attending", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.AssignAttendingClinicianTestee,
			)...)
			relationAdmin.POST("/assign-collaborator", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.AssignCollaboratorClinicianTestee,
			)...)
			relationAdmin.POST("/transfer-primary", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.TransferPrimaryClinicianTestee,
			)...)
			relationAdmin.POST("/:id/unbind", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				operatorClinicianHandler.UnbindClinicianTesteeRelation,
			)...)
		}
	}

	if assessmentEntryHandler := r.deps.Actor.AssessmentEntryHandler; assessmentEntryHandler != nil {
		assessmentEntries := apiV1.Group("/assessment-entries", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		{
			assessmentEntries.GET("/:id", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.QueryGlobalQPS,
				r.rateCfg.QueryGlobalBurst,
				r.rateCfg.QueryUserQPS,
				r.rateCfg.QueryUserBurst,
				assessmentEntryHandler.GetAssessmentEntry,
			)...)
			assessmentEntries.POST("/:id/deactivate", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				assessmentEntryHandler.DeactivateAssessmentEntry,
			)...)
			assessmentEntries.POST("/:id/reactivate", r.rateLimitedHandlers(
				r.rateCfg,
				r.rateCfg.SubmitGlobalQPS,
				r.rateCfg.SubmitGlobalBurst,
				r.rateCfg.SubmitUserQPS,
				r.rateCfg.SubmitUserBurst,
				assessmentEntryHandler.ReactivateAssessmentEntry,
			)...)
		}
	}
}

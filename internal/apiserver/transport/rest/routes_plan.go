package rest

import (
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/gin-gonic/gin"
)

func (r *Router) newPlanHandler() *handler.PlanHandler {
	if r.deps.Plan.CommandService == nil || r.deps.Plan.QueryService == nil {
		return nil
	}
	planHandler := handler.NewPlanHandler(r.deps.Plan.CommandService, r.deps.Plan.QueryService)
	if r.deps.Plan.TesteeAccessService != nil {
		planHandler.SetTesteeAccessService(r.deps.Plan.TesteeAccessService)
	}
	return planHandler
}

// registerPlanProtectedRoutes 注册 Plan 模块相关的受保护路由。
func (r *Router) registerPlanProtectedRoutes(apiV1 *gin.RouterGroup) {
	planHandler := r.newPlanHandler()
	if planHandler == nil {
		return
	}

	plans := apiV1.Group("/plans")
	{
		plans.POST("", withPermission(authzapp.EvaluationPlanResource, "create", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.CreatePlan)...)...)
		plans.POST("/:id/pause", withPermission(authzapp.EvaluationPlanResource, "pause", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.PausePlan)...)...)
		plans.POST("/:id/resume", withPermission(authzapp.EvaluationPlanResource, "resume", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.ResumePlan)...)...)
		plans.POST("/:id/finish", withPermission(authzapp.EvaluationPlanResource, "update", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.FinishPlan)...)...)
		plans.POST("/:id/cancel", withPermission(authzapp.EvaluationPlanResource, "cancel", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.CancelPlan)...)...)

		plans.GET("", withPermission(authzapp.EvaluationPlanResource, "list", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListPlans)...)...)
		plans.GET("/:id/tasks", withPermission(authzapp.EvaluationPlanTaskResource, "list", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListTasksByPlan)...)...)
		plans.GET("/:id", withPermission(authzapp.EvaluationPlanResource, "read", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.GetPlan)...)...)

		plans.POST("/enroll", withPermission(authzapp.EvaluationPlanResource, "enroll", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.EnrollTestee)...)...)
		plans.POST("/:id/testees/:testee_id/terminate", withPermission(authzapp.EvaluationPlanResource, "terminate", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.TerminateEnrollment)...)...)
	}

	tasks := apiV1.Group("/plans/tasks")
	{
		tasks.GET("", withPermission(authzapp.EvaluationPlanTaskResource, "list", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListTasks)...)...)
		tasks.GET("/:id", withPermission(authzapp.EvaluationPlanTaskResource, "read", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.GetTask)...)...)
		tasks.POST("/:id/open", withPermission(authzapp.EvaluationPlanTaskResource, "open", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.OpenTask)...)...)
		tasks.POST("/:id/cancel", withPermission(authzapp.EvaluationPlanTaskResource, "cancel", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.CancelTask)...)...)
	}

	testees := apiV1.Group("/testees")
	{
		testees.GET("/:id/plans/:plan_id/tasks", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListTasksByTesteeAndPlan)...)
		testees.GET("/:id/plans", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListPlansByTestee)...)
		testees.GET("/:id/tasks", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListTasksByTestee)...)
	}
}

func (r *Router) registerPlanV2ProtectedRoutes(apiV2 *gin.RouterGroup) {
	if r.deps.Plan.EnrollmentQueryService == nil {
		return
	}
	h := handler.NewPlanEnrollmentQueryHandler(r.deps.Plan.EnrollmentQueryService, r.deps.Plan.TesteeAccessService)
	apiV2.GET("/plans/testees/:testee_id/enrollments", r.rateLimitedHandlers(rateLimitBudgetQuery, h.List)...)
}

func (r *Router) registerPlanInternalRoutes(internalV1 *gin.RouterGroup) {
	planHandler := r.newPlanHandler()
	if planHandler == nil {
		return
	}

	tasks := internalV1.Group("/plans/tasks")
	tasks.POST("/schedule", withPermission(authzapp.EvaluationPlanTaskResource, "schedule", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.SchedulePendingTasks)...)...)
	tasks.POST("/window", withPermission(authzapp.EvaluationPlanTaskResource, "list", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListTaskWindow)...)...)
	tasks.POST("/:id/complete", withPermission(authzapp.EvaluationPlanTaskResource, "complete", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.CompleteTask)...)...)
	tasks.POST("/:id/expire", withPermission(authzapp.EvaluationPlanTaskResource, "expire", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.ExpireTask)...)...)
}

package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
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
		planWrites := plans.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
		planWrites.POST("", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.CreatePlan)...)
		planWrites.POST("/:id/pause", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.PausePlan)...)
		planWrites.POST("/:id/resume", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.ResumePlan)...)
		planWrites.POST("/:id/finish", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.FinishPlan)...)
		planWrites.POST("/:id/cancel", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.CancelPlan)...)

		plans.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListPlans)...)
		plans.GET("/:id/tasks", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListTasksByPlan)...)
		plans.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.GetPlan)...)

		planWrites.POST("/enroll", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.EnrollTestee)...)
		planWrites.POST("/:id/testees/:testee_id/terminate", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.TerminateEnrollment)...)
	}

	tasks := apiV1.Group("/plans/tasks")
	{
		taskWrites := tasks.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
		tasks.GET("", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListTasks)...)
		tasks.GET("/:id", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.GetTask)...)
		taskWrites.POST("/:id/open", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.OpenTask)...)
		taskWrites.POST("/:id/cancel", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.CancelTask)...)
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

	tasks := internalV1.Group("/plans/tasks", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
	tasks.POST("/schedule", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.SchedulePendingTasks)...)
	tasks.POST("/window", r.rateLimitedHandlers(rateLimitBudgetQuery, planHandler.ListTaskWindow)...)
	tasks.POST("/:id/complete", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.CompleteTask)...)
	tasks.POST("/:id/expire", r.rateLimitedHandlers(rateLimitBudgetSubmit, planHandler.ExpireTask)...)
}

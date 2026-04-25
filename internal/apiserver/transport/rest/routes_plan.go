package rest

import (
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

// registerPlanProtectedRoutes 注册 Plan 模块相关的受保护路由。
func (r *Router) registerPlanProtectedRoutes(apiV1 *gin.RouterGroup) {
	planHandler := r.deps.Plan.Handler
	if planHandler == nil {
		return
	}

	plans := apiV1.Group("/plans")
	{
		planWrites := plans.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
		planWrites.POST("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.CreatePlan,
		)...)
		planWrites.POST("/:id/pause", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.PausePlan,
		)...)
		planWrites.POST("/:id/resume", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.ResumePlan,
		)...)
		planWrites.POST("/:id/finish", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.FinishPlan,
		)...)
		planWrites.POST("/:id/cancel", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.CancelPlan,
		)...)

		plans.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListPlans,
		)...)
		plans.GET("/:id/tasks", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListTasksByPlan,
		)...)
		plans.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.GetPlan,
		)...)

		planWrites.POST("/enroll", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.EnrollTestee,
		)...)
		planWrites.POST("/:id/testees/:testee_id/terminate", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.TerminateEnrollment,
		)...)
	}

	tasks := apiV1.Group("/plans/tasks")
	{
		taskWrites := tasks.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
		tasks.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListTasks,
		)...)
		tasks.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.GetTask,
		)...)
		taskWrites.POST("/:id/open", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.OpenTask,
		)...)
		taskWrites.POST("/:id/cancel", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.SubmitGlobalQPS,
			r.rateCfg.SubmitGlobalBurst,
			r.rateCfg.SubmitUserQPS,
			r.rateCfg.SubmitUserBurst,
			planHandler.CancelTask,
		)...)
	}

	testees := apiV1.Group("/testees")
	{
		testees.GET("/:id/plans/:plan_id/tasks", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListTasksByTesteeAndPlan,
		)...)
		testees.GET("/:id/plans", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListPlansByTestee,
		)...)
		testees.GET("/:id/tasks", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			planHandler.ListTasksByTestee,
		)...)
	}
}

func (r *Router) registerPlanInternalRoutes(internalV1 *gin.RouterGroup) {
	planHandler := r.deps.Plan.Handler
	if planHandler == nil {
		return
	}

	tasks := internalV1.Group("/plans/tasks", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageEvaluationPlans))
	tasks.POST("/schedule", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		planHandler.SchedulePendingTasks,
	)...)
	tasks.POST("/window", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		planHandler.ListTaskWindow,
	)...)
	tasks.POST("/:id/complete", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		planHandler.CompleteTask,
	)...)
	tasks.POST("/:id/expire", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.SubmitGlobalQPS,
		r.rateCfg.SubmitGlobalBurst,
		r.rateCfg.SubmitUserQPS,
		r.rateCfg.SubmitUserBurst,
		planHandler.ExpireTask,
	)...)
}

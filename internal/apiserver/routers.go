package apiserver

import (
	"fmt"
	"net/http"

	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// Router 集中的路由管理器
type Router struct {
	container *container.Container
}

// NewRouter 创建路由管理器
func NewRouter(c *container.Container) *Router {
	return &Router{
		container: c,
	}
}

// RegisterRoutes 注册所有路由
func (r *Router) RegisterRoutes(engine *gin.Engine) {
	// OpenAPI 契约（OAS 3.1）与 UI
	engine.Static("/api/rest", "./api/rest")
	engine.Static("/swagger-ui", "./web/swagger-ui/swagger-ui-dist")
	// 兼容入口
	engine.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger-ui/")
	})

	// 注册公开路由（不需要认证）
	r.registerPublicRoutes(engine)

	// 注册需要认证的路由
	r.registerProtectedRoutes(engine)

	fmt.Printf("🔗 Registered routes for: public, protected(user, questionnaire)\n")
}

// registerPublicRoutes 注册公开路由（不需要认证）
func (r *Router) registerPublicRoutes(engine *gin.Engine) {
	// 健康检查和基础路由
	engine.GET("/health", r.healthCheck)
	engine.GET("/ping", r.ping)

	// 认证相关的公开路由 已迁移至 IAM / API 网关，不在此维护

	// 公开的API路由
	publicAPI := engine.Group("/api/v1/public")
	{
		publicAPI.GET("/info", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"service":     "questionnaire-scale",
				"version":     "1.0.0",
				"description": "问卷量表管理系统",
			})
		})
	}
}

// registerProtectedRoutes 注册需要认证的路由
func (r *Router) registerProtectedRoutes(engine *gin.Engine) {
	// 创建需要认证的API组
	apiV1 := engine.Group("/api/v1")

	// 应用 IAM JWT 认证中间件（如果启用，使用 SDK TokenVerifier 本地验签）
	if r.container.IAMModule != nil && r.container.IAMModule.IsEnabled() {
		tokenVerifier := r.container.IAMModule.SDKTokenVerifier()
		if tokenVerifier != nil {
			apiV1.Use(middleware.JWTAuthMiddleware(tokenVerifier))
			// 添加用户身份解析中间件：从 JWT claims 提取 UserID、OrgID、Roles
			apiV1.Use(restmiddleware.UserIdentityMiddleware())
			fmt.Printf("🔐 JWT authentication middleware enabled for /api/v1 (local JWKS verification)\n")
		} else {
			fmt.Printf("⚠️  Warning: TokenVerifier not available, JWT authentication disabled!\n")
		}
	} else {
		fmt.Printf("⚠️  Warning: IAM authentication is disabled, routes are unprotected!\n")
	}

	// 注册用户相关的受保护路由
	r.registerUserProtectedRoutes(apiV1)

	// 注册问卷相关的受保护路由
	r.registerQuestionnaireProtectedRoutes(apiV1)

	// 注册答卷相关的受保护路由
	r.registerAnswersheetProtectedRoutes(apiV1)

	// 注册量表相关的受保护路由
	r.registerScaleProtectedRoutes(apiV1)

	// 注册 Evaluation 模块相关的受保护路由
	r.registerEvaluationProtectedRoutes(apiV1)

	// 注册 Plan 模块相关的受保护路由（必须在 registerActorProtectedRoutes 之前，确保更具体的路由先注册）
	r.registerPlanProtectedRoutes(apiV1)

	// 注册 Statistics 模块相关的受保护路由
	r.registerStatisticsProtectedRoutes(apiV1)

	// 注册 Actor 模块相关的受保护路由
	r.registerActorProtectedRoutes(apiV1)

	// 注册 Codes 申请路由
	r.registerCodesRoutes(apiV1)

	// 管理员路由（需要额外的权限检查）
	r.registerAdminRoutes(apiV1)
}

// registerUserProtectedRoutes 注册用户相关的受保护路由
// 用户管理已迁移到 IAM 服务，此方法保留以便未来扩展
func (r *Router) registerUserProtectedRoutes(apiV1 *gin.RouterGroup) {
	// 用户相关功能已迁移到 iam-contracts 项目
}

// registerCodesRoutes 注册 codes 申请路由
func (r *Router) registerCodesRoutes(apiV1 *gin.RouterGroup) {
	if r.container == nil {
		return
	}

	if r.container.CodesService == nil {
		return
	}

	handler := codesHandler.NewCodesHandler(r.container.CodesService)
	apiV1.POST("/codes/apply", handler.Apply)
}

// registerQuestionnaireProtectedRoutes 注册问卷相关的受保护路由
func (r *Router) registerQuestionnaireProtectedRoutes(apiV1 *gin.RouterGroup) {
	quesHandler := r.container.SurveyModule.Questionnaire.Handler
	if quesHandler == nil {
		return
	}

	questionnaires := apiV1.Group("/questionnaires")
	{
		// 生命周期管理
		questionnaires.POST("", quesHandler.Create)                          // 创建问卷
		questionnaires.PUT("/:code/basic-info", quesHandler.UpdateBasicInfo) // 更新基本信息
		questionnaires.POST("/:code/draft", quesHandler.SaveDraft)           // 保存草稿
		questionnaires.POST("/:code/publish", quesHandler.Publish)           // 发布问卷
		questionnaires.POST("/:code/unpublish", quesHandler.Unpublish)       // 取消发布
		questionnaires.POST("/:code/archive", quesHandler.Archive)           // 归档问卷
		questionnaires.DELETE("/:code", quesHandler.Delete)                  // 删除问卷

		// 问题内容管理
		questionnaires.POST("/:code/questions", quesHandler.AddQuestion)               // 添加问题
		questionnaires.PUT("/:code/questions/:qcode", quesHandler.UpdateQuestion)      // 更新问题
		questionnaires.DELETE("/:code/questions/:qcode", quesHandler.RemoveQuestion)   // 删除问题
		questionnaires.POST("/:code/questions/reorder", quesHandler.ReorderQuestions)  // 重排问题
		questionnaires.PUT("/:code/questions/batch", quesHandler.BatchUpdateQuestions) // 批量更新

		// 查询接口
		questionnaires.GET("/:code", quesHandler.GetByCode)                    // 获取问卷详情
		questionnaires.GET("", quesHandler.List)                               // 获取问卷列表
		questionnaires.GET("/published/:code", quesHandler.GetPublishedByCode) // 获取已发布问卷
		questionnaires.GET("/published", quesHandler.ListPublished)            // 获取已发布列表
	}
}

// registerAnswersheetProtectedRoutes 注册答卷相关的受保护路由
func (r *Router) registerAnswersheetProtectedRoutes(apiV1 *gin.RouterGroup) {
	answersheetHandler := r.container.SurveyModule.AnswerSheet.Handler
	if answersheetHandler == nil {
		return
	}

	answersheets := apiV1.Group("/answersheets")
	{
		// 管理接口
		answersheets.GET("/:id", answersheetHandler.GetByID) // 获取答卷详情
		answersheets.GET("", answersheetHandler.List)        // 获取答卷列表
		// 统计接口已迁移到 /api/v1/statistics/questionnaires/:code
	}
}

// registerScaleProtectedRoutes 注册量表相关的受保护路由
func (r *Router) registerScaleProtectedRoutes(apiV1 *gin.RouterGroup) {
	scaleHandler := r.container.ScaleModule.Handler
	if scaleHandler == nil {
		return
	}

	scales := apiV1.Group("/scales")
	{
		// 生命周期管理
		scales.POST("", scaleHandler.Create)                                 // 创建量表
		scales.PUT("/:code/basic-info", scaleHandler.UpdateBasicInfo)        // 更新基本信息
		scales.PUT("/:code/questionnaire", scaleHandler.UpdateQuestionnaire) // 更新关联问卷
		scales.POST("/:code/publish", scaleHandler.Publish)                  // 发布量表
		scales.POST("/:code/unpublish", scaleHandler.Unpublish)              // 下架量表
		scales.POST("/:code/archive", scaleHandler.Archive)                  // 归档量表
		scales.DELETE("/:code", scaleHandler.Delete)                         // 删除量表

		// 因子管理（仅提供批量操作）
		scales.PUT("/:code/factors/batch", scaleHandler.BatchUpdateFactors)      // 批量更新因子
		scales.PUT("/:code/interpret-rules", scaleHandler.ReplaceInterpretRules) // 批量设置解读规则

		// 查询接口（注意：具体路径要放在参数路径之前，避免路由冲突）
		scales.GET("/categories", scaleHandler.GetCategories)                // 获取量表分类列表
		scales.GET("/by-questionnaire", scaleHandler.GetByQuestionnaireCode) // 根据问卷获取量表
		scales.GET("/published/:code", scaleHandler.GetPublishedByCode)      // 获取已发布量表
		scales.GET("/published", scaleHandler.ListPublished)                 // 获取已发布列表
		scales.GET("/:code/factors", scaleHandler.GetFactors)                // 获取因子列表
		scales.GET("/:code", scaleHandler.GetByCode)                         // 获取量表详情
		scales.GET("", scaleHandler.List)                                    // 获取量表列表
	}
}

// registerActorProtectedRoutes 注册 Actor 模块相关的受保护路由
func (r *Router) registerActorProtectedRoutes(apiV1 *gin.RouterGroup) {
	actorHandler := r.container.ActorModule.ActorHandler
	if actorHandler == nil {
		return
	}

	// 受试者路由
	testees := apiV1.Group("/testees")
	{
		testees.GET("", actorHandler.ListTestees)                         // 查询受试者列表
		testees.GET("/:id", actorHandler.GetTestee)                       // 获取受试者详情
		testees.PUT("/:id", actorHandler.UpdateTestee)                    // 更新受试者
		testees.GET("/:id/scale-analysis", actorHandler.GetScaleAnalysis) // 受试者量表分析
		// 统计接口已迁移到 /api/v1/statistics/testees/:testee_id
	}

	// 员工路由
	staff := apiV1.Group("/staff")
	{
		staff.POST("", actorHandler.CreateStaff)       // 创建员工
		staff.GET("", actorHandler.ListStaff)          // 查询员工列表
		staff.GET("/:id", actorHandler.GetStaff)       // 获取员工详情
		staff.DELETE("/:id", actorHandler.DeleteStaff) // 删除员工
	}
}

// registerEvaluationProtectedRoutes 注册评估模块相关的受保护路由
func (r *Router) registerEvaluationProtectedRoutes(apiV1 *gin.RouterGroup) {
	evalHandler := r.container.EvaluationModule.Handler
	if evalHandler == nil {
		return
	}

	evaluations := apiV1.Group("/evaluations")
	{
		// ==================== Assessment 查询路由（后台管理）====================
		assessments := evaluations.Group("/assessments")
		{
			// 查询
			assessments.GET("", evalHandler.ListAssessments)   // 查询测评列表
			assessments.GET("/:id", evalHandler.GetAssessment) // 获取测评详情
			// 统计接口已迁移到 /api/v1/statistics/questionnaires/:code 或 /api/v1/statistics/system

			// 得分和报告
			assessments.GET("/:id/scores", evalHandler.GetScores)                     // 获取测评得分
			assessments.GET("/:id/report", evalHandler.GetReport)                     // 获取测评报告
			assessments.GET("/:id/high-risk-factors", evalHandler.GetHighRiskFactors) // 获取高风险因子

			// 管理操作
			assessments.POST("/:id/retry", evalHandler.RetryFailed) // 重试失败的测评
		}

		// ==================== Score 相关路由 ====================
		scores := evaluations.Group("/scores")
		{
			scores.GET("/trend", evalHandler.GetFactorTrend) // 获取因子趋势
		}

		// ==================== Report 相关路由 ====================
		reports := evaluations.Group("/reports")
		{
			reports.GET("", evalHandler.ListReports) // 查询报告列表
		}

		// ==================== 批量操作路由 ====================
		evaluations.POST("/batch-evaluate", evalHandler.BatchEvaluate) // 批量评估
	}
}

// registerPlanProtectedRoutes 注册 Plan 模块相关的受保护路由
func (r *Router) registerPlanProtectedRoutes(apiV1 *gin.RouterGroup) {
	planHandler := r.container.PlanModule.Handler
	if planHandler == nil {
		return
	}

	plans := apiV1.Group("/plans")
	{
		// ==================== Plan 生命周期管理 ====================
		plans.POST("", planHandler.CreatePlan)            // 创建计划
		plans.POST("/:id/pause", planHandler.PausePlan)   // 暂停计划
		plans.POST("/:id/resume", planHandler.ResumePlan) // 恢复计划
		plans.POST("/:id/cancel", planHandler.CancelPlan) // 取消计划

		// ==================== Plan 查询 ====================
		plans.GET("", planHandler.ListPlans)                 // 查询计划列表
		plans.GET("/:id/tasks", planHandler.ListTasksByPlan) // 查询计划下的所有任务（必须在 /:id 之前注册）
		plans.GET("/:id", planHandler.GetPlan)               // 获取计划详情

		// ==================== Plan 受试者管理 ====================
		plans.POST("/enroll", planHandler.EnrollTestee)                                  // 受试者加入计划
		plans.POST("/:id/testees/:testee_id/terminate", planHandler.TerminateEnrollment) // 终止受试者的计划参与
	}

	// ==================== Task 管理 ====================
	tasks := apiV1.Group("/plans/tasks")
	{
		tasks.POST("/schedule", planHandler.SchedulePendingTasks) // 调度待推送任务
		tasks.GET("", planHandler.ListTasks)                      // 查询任务列表
		tasks.GET("/:id", planHandler.GetTask)                    // 获取任务详情
		tasks.POST("/:id/open", planHandler.OpenTask)             // 开放任务
		tasks.POST("/:id/complete", planHandler.CompleteTask)     // 完成任务
		tasks.POST("/:id/expire", planHandler.ExpireTask)         // 过期任务
		tasks.POST("/:id/cancel", planHandler.CancelTask)         // 取消任务
	}

	// ==================== Testee 相关的 Plan 查询 ====================
	// 注意：这些路由必须在 registerActorProtectedRoutes 之后注册，且更具体的路由要放在前面
	testees := apiV1.Group("/testees")
	{
		testees.GET("/:id/plans/:plan_id/tasks", planHandler.ListTasksByTesteeAndPlan) // 查询受试者在某个计划下的所有任务（最具体，最先匹配）
		testees.GET("/:id/plans", planHandler.ListPlansByTestee)                       // 查询受试者参与的所有计划
		testees.GET("/:id/tasks", planHandler.ListTasksByTestee)                       // 查询受试者的所有任务
	}
}

// registerStatisticsProtectedRoutes 注册 Statistics 模块相关的受保护路由
func (r *Router) registerStatisticsProtectedRoutes(apiV1 *gin.RouterGroup) {
	statisticsModule := r.container.StatisticsModule
	if statisticsModule == nil || statisticsModule.Handler == nil {
		return
	}

	statistics := apiV1.Group("/statistics")
	{
		// ==================== 统计查询 ====================
		statistics.GET("/system", statisticsModule.Handler.GetSystemStatistics)                      // 获取系统整体统计
		statistics.GET("/questionnaires/:code", statisticsModule.Handler.GetQuestionnaireStatistics) // 获取问卷/量表统计
		statistics.GET("/testees/:testee_id", statisticsModule.Handler.GetTesteeStatistics)          // 获取受试者统计
		statistics.GET("/plans/:plan_id", statisticsModule.Handler.GetPlanStatistics)                // 获取计划统计

		// ==================== 定时任务接口 ====================
		sync := statistics.Group("/sync")
		{
			sync.POST("/daily", statisticsModule.Handler.SyncDailyStatistics)             // 同步每日统计
			sync.POST("/accumulated", statisticsModule.Handler.SyncAccumulatedStatistics) // 同步累计统计
			sync.POST("/plan", statisticsModule.Handler.SyncPlanStatistics)               // 同步计划统计
		}

		// ==================== 数据校验 ====================
		statistics.POST("/validate", statisticsModule.Handler.ValidateConsistency) // 校验数据一致性
	}
}

// registerAdminRoutes 注册管理员路由
func (r *Router) registerAdminRoutes(apiV1 *gin.RouterGroup) {
	admin := apiV1.Group("/admin")
	// admin.Use(r.requireAdminRole()) // 需要实现管理员权限检查中间件
	{
		admin.GET("/users", r.placeholder)      // 管理员获取所有用户
		admin.GET("/statistics", r.placeholder) // 系统统计信息
		admin.GET("/logs", r.placeholder)       // 系统日志
	}
}

// placeholder 占位符处理器（用于未实现的功能）
func (r *Router) placeholder(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    501,
		"message": "功能尚未实现",
		"path":    c.Request.URL.Path,
		"method":  c.Request.Method,
	})
}

// healthCheck 健康检查处理函数
// @Summary 健康检查
// @Description 检查 API Server 健康状态
// @Tags 系统
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
func (r *Router) healthCheck(c *gin.Context) {
	response := gin.H{
		"status":       "healthy",
		"version":      "1.0.0",
		"discovery":    "auto",
		"architecture": "hexagonal",
		"router":       "centralized",
		"auth":         "delegated", // 认证由 IAM / API 网关代理
		"components": gin.H{
			"domain":      "questionnaire",
			"ports":       "storage",
			"adapters":    "mysql, mongodb, http",
			"application": "questionnaire_service",
		},
		// JWT 配置移除（由 IAM 管理）
	}

	c.JSON(200, response)
}

// ping 简单的连通性测试
// @Summary Ping
// @Description 测试 API Server 连通性
// @Tags 系统
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /ping [get]
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
		"status":  "ok",
		"router":  "centralized",
		"auth":    "enabled",
	})
}

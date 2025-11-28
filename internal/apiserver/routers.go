package apiserver

import (
	"fmt"
	"net/http"

	"github.com/FangcunMount/qs-server/internal/apiserver/container"
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

	// 认证由上游网关或 IAM 负责，这里不再强制中间件

	// 注册用户相关的受保护路由
	r.registerUserProtectedRoutes(apiV1)

	// 注册问卷相关的受保护路由
	r.registerQuestionnaireProtectedRoutes(apiV1)

	// 注册答卷相关的受保护路由
	r.registerAnswersheetProtectedRoutes(apiV1)

	// 注册医学量表相关的受保护路由（旧版，待废弃）
	r.registerMedicalScaleProtectedRoutes(apiV1)

	// 注册量表相关的受保护路由（重构版）
	r.registerScaleProtectedRoutes(apiV1)

	// 注册 Actor 模块相关的受保护路由
	r.registerActorProtectedRoutes(apiV1)

	// 管理员路由（需要额外的权限检查）
	r.registerAdminRoutes(apiV1)
}

// registerUserProtectedRoutes 注册用户相关的受保护路由
// 用户管理已迁移到 IAM 服务，此方法保留以便未来扩展
func (r *Router) registerUserProtectedRoutes(apiV1 *gin.RouterGroup) {
	// 用户相关功能已迁移到 iam-contracts 项目
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
		questionnaires.POST("", quesHandler.Create)                    // 创建问卷
		questionnaires.PUT("/:code", quesHandler.UpdateBasicInfo)      // 更新基本信息
		questionnaires.POST("/:code/draft", quesHandler.SaveDraft)     // 保存草稿
		questionnaires.POST("/:code/publish", quesHandler.Publish)     // 发布问卷
		questionnaires.POST("/:code/unpublish", quesHandler.Unpublish) // 取消发布
		questionnaires.POST("/:code/archive", quesHandler.Archive)     // 归档问卷
		questionnaires.DELETE("/:code", quesHandler.Delete)            // 删除问卷

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
		answersheets.GET("/:id", answersheetHandler.GetByID)              // 获取答卷详情
		answersheets.GET("", answersheetHandler.List)                     // 获取答卷列表
		answersheets.DELETE("/:id", answersheetHandler.Delete)            // 删除答卷
		answersheets.GET("/statistics", answersheetHandler.GetStatistics) // 获取统计信息

		// 评分接口
		answersheets.PUT("/:id/score", answersheetHandler.UpdateScore) // 更新分数
	}
}

// registerMedicalScaleProtectedRoutes 注册医学量表相关的受保护路由
func (r *Router) registerMedicalScaleProtectedRoutes(apiV1 *gin.RouterGroup) {
	medicalScaleHandler := r.container.MedicalScaleModule.MSHandler
	if medicalScaleHandler == nil {
		return
	}

	medicalScales := apiV1.Group("/medical-scales")
	{
		medicalScales.POST("", medicalScaleHandler.Create)
		medicalScales.GET("/:code", medicalScaleHandler.Get)
		medicalScales.PUT("/:code", medicalScaleHandler.UpdateBaseInfo)
		medicalScales.PUT("/:code/factors", medicalScaleHandler.UpdateFactor)
	}
}

// registerScaleProtectedRoutes 注册量表相关的受保护路由（重构版）
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
		scales.PUT("/:code/factors", scaleHandler.ReplaceFactors)                // 批量替换因子
		scales.PUT("/:code/interpret-rules", scaleHandler.ReplaceInterpretRules) // 批量设置解读规则

		// 查询接口
		scales.GET("/:code", scaleHandler.GetByCode)                         // 获取量表详情
		scales.GET("", scaleHandler.List)                                    // 获取量表列表
		scales.GET("/by-questionnaire", scaleHandler.GetByQuestionnaireCode) // 根据问卷获取量表
		scales.GET("/published/:code", scaleHandler.GetPublishedByCode)      // 获取已发布量表
		scales.GET("/published", scaleHandler.ListPublished)                 // 获取已发布列表
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
		testees.GET("", actorHandler.ListTestees)      // 查询受试者列表
		testees.GET("/:id", actorHandler.GetTestee)    // 获取受试者详情
		testees.PUT("/:id", actorHandler.UpdateTestee) // 更新受试者
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
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
		"status":  "ok",
		"router":  "centralized",
		"auth":    "enabled",
	})
}

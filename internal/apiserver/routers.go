package apiserver

import (
	"fmt"
	"net/http"

	"github.com/fangcun-mount/qs-server/internal/apiserver/container"
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

	// 注册医学量表相关的受保护路由
	r.registerMedicalScaleProtectedRoutes(apiV1)

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
	quesHandler := r.container.QuestionnaireModule.QuesHandler
	if quesHandler == nil {
		return
	}

	questionnaires := apiV1.Group("/questionnaires")
	{
		// 问卷CRUD操作
		questionnaires.POST("", quesHandler.CreateQuestionnaire) // 创建问卷
		questionnaires.GET("", quesHandler.QueryList)            // 获取问卷列表
		questionnaires.GET("/:code", quesHandler.QueryOne)       // 获取指定问卷
		questionnaires.PUT("/:code", quesHandler.EditBasicInfo)  // 更新问卷

		// 问卷状态管理
		questionnaires.POST("/:code/publish", quesHandler.PublishQuestionnaire)   // 发布问卷
		questionnaires.POST("/:code/archive", quesHandler.UnpublishQuestionnaire) // 归档问卷

		// 问卷问题管理
		questionnaires.PUT("/:code/questions", quesHandler.UpdateQuestions) // 更新问卷问题
	}
}

// registerAnswersheetProtectedRoutes 注册答卷相关的受保护路由
func (r *Router) registerAnswersheetProtectedRoutes(apiV1 *gin.RouterGroup) {
	answersheetHandler := r.container.AnswersheetModule.AnswersheetHandler
	if answersheetHandler == nil {
		return
	}

	answersheets := apiV1.Group("/answersheets")
	{
		answersheets.POST("", answersheetHandler.Save)   // 保存答卷
		answersheets.GET("/:id", answersheetHandler.Get) // 获取答卷
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

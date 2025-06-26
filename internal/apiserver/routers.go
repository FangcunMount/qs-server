package apiserver

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers/user"
)

// Router 集中的路由管理器
type Router struct {
	// handlers
	userHandler          *user.Handler
	questionnaireHandler *questionnaire.Handler

	// container reference for health check
	container *AutoDiscoveryContainer
}

// NewRouter 创建路由管理器
func NewRouter() *Router {
	return &Router{}
}

// SetContainer 设置容器引用（用于健康检查）
func (r *Router) SetContainer(container *AutoDiscoveryContainer) {
	r.container = container
}

// SetUserHandler 设置用户处理器
func (r *Router) SetUserHandler(handler *user.Handler) {
	r.userHandler = handler
}

// SetQuestionnaireHandler 设置问卷处理器
func (r *Router) SetQuestionnaireHandler(handler *questionnaire.Handler) {
	r.questionnaireHandler = handler
}

// RegisterRoutes 注册所有路由
func (r *Router) RegisterRoutes(engine *gin.Engine) {
	// 安装中间件
	r.installMiddleware(engine)

	// 注册健康检查路由
	r.registerHealthRoutes(engine)

	// API版本组
	apiV1 := engine.Group("/api/v1")

	// 注册业务路由
	r.registerUserRoutes(apiV1)
	r.registerQuestionnaireRoutes(apiV1)

	fmt.Printf("🔗 Registered routes for: user, questionnaire\n")
}

// installMiddleware 安装中间件
func (r *Router) installMiddleware(engine *gin.Engine) {
	engine.Use(gin.Recovery())
	engine.Use(gin.Logger())

	// TODO: 可以在这里添加更多中间件
	// engine.Use(cors.Default())
	// engine.Use(ratelimit.RateLimiter(...))
}

// registerHealthRoutes 注册健康检查路由
func (r *Router) registerHealthRoutes(engine *gin.Engine) {
	engine.GET("/health", r.healthCheck)
	engine.GET("/ping", r.ping)
}

// registerUserRoutes 注册用户相关路由
func (r *Router) registerUserRoutes(apiV1 *gin.RouterGroup) {
	if r.userHandler == nil {
		return
	}

	users := apiV1.Group("/users")
	{
		users.POST("", r.userHandler.CreateUser)
		users.GET("/:id", r.userHandler.GetUser)
		users.GET("", r.userHandler.ListUsers)
		users.PUT("/:id", r.userHandler.UpdateUser)
		users.DELETE("/:id", r.userHandler.DeleteUser)
		users.POST("/:id/activate", r.userHandler.ActivateUser)
		users.POST("/:id/block", r.userHandler.BlockUser)

		// 新增路由
		users.PUT("/:id/password", r.userHandler.ChangePassword)
		users.GET("/active", r.userHandler.GetActiveUsers)
	}
}

// registerQuestionnaireRoutes 注册问卷相关路由
func (r *Router) registerQuestionnaireRoutes(apiV1 *gin.RouterGroup) {
	if r.questionnaireHandler == nil {
		return
	}

	questionnaires := apiV1.Group("/questionnaires")
	{
		questionnaires.POST("", r.questionnaireHandler.CreateQuestionnaire)
		questionnaires.GET("", r.questionnaireHandler.GetQuestionnaire)
		questionnaires.GET("/list", r.questionnaireHandler.ListQuestionnaires)
		questionnaires.PUT("/:id", r.questionnaireHandler.UpdateQuestionnaire)
		questionnaires.POST("/:id/publish", r.questionnaireHandler.PublishQuestionnaire)
		questionnaires.DELETE("/:id", r.questionnaireHandler.DeleteQuestionnaire)

		// 数据一致性相关路由
		questionnaires.GET("/:id/consistency", r.questionnaireHandler.CheckDataConsistency)
		questionnaires.POST("/:id/repair", r.questionnaireHandler.RepairData)
	}
}

// healthCheck 健康检查处理函数
func (r *Router) healthCheck(c *gin.Context) {
	response := gin.H{
		"status":       "healthy",
		"version":      "1.0.0",
		"discovery":    "auto",
		"architecture": "hexagonal",
		"router":       "centralized",
		"components": gin.H{
			"domain":      "questionnaire, user",
			"ports":       "storage",
			"adapters":    "mysql, mongodb, http",
			"application": "questionnaire_service, user_service",
		},
	}

	// 如果有容器引用，添加更详细的信息
	if r.container != nil {
		response["repositories"] = r.container.getRegisteredRepositories()
		response["services"] = r.container.getRegisteredServices()
		response["handlers"] = r.container.getRegisteredHandlers()
	}

	c.JSON(200, response)
}

// ping 简单的连通性测试
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
		"status":  "ok",
		"router":  "centralized",
	})
}

// RegisterCustomRoutes 注册自定义路由（扩展点）
func (r *Router) RegisterCustomRoutes(apiV1 *gin.RouterGroup, routerFunc func(*gin.RouterGroup)) {
	if routerFunc != nil {
		routerFunc(apiV1)
	}
}

// 未来扩展示例：
// registerScaleRoutes 注册量表相关路由（示例）
// func (r *Router) registerScaleRoutes(apiV1 *gin.RouterGroup) {
//     if r.scaleHandler == nil {
//         return
//     }
//
//     scales := apiV1.Group("/scales")
//     {
//         scales.POST("", r.scaleHandler.CreateScale)
//         scales.GET("/:id", r.scaleHandler.GetScale)
//         scales.GET("", r.scaleHandler.ListScales)
//         scales.PUT("/:id", r.scaleHandler.UpdateScale)
//         scales.DELETE("/:id", r.scaleHandler.DeleteScale)
//     }
// }

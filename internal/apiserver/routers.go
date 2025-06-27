package apiserver

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/container"
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
	// 安装中间件
	r.installMiddleware(engine)

	// 注册健康检查路由
	r.registerHealthRoutes(engine)

	// API版本组
	apiV1 := engine.Group("/api/v1")

	// 注册业务路由
	r.registerUserRoutes(apiV1)
	// r.registerQuestionnaireRoutes(apiV1)

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
	userHandler := r.container.GetUserModule().GetHandler()
	if userHandler == nil {
		return
	}

	users := apiV1.Group("/users")
	{
		users.POST("", userHandler.CreateUser)
		users.GET("/:id", userHandler.GetUser)
		users.PUT("/:id", userHandler.UpdateUser)
	}
}

// registerQuestionnaireRoutes 注册问卷相关路由
func (r *Router) registerQuestionnaireRoutes(apiV1 *gin.RouterGroup) {
	// TODO: 待实现
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

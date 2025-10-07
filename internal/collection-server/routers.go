package collection

import (
	"fmt"
	"net/http"

	"github.com/fangcun-mount/qs-server/internal/collection-server/container"
	"github.com/fangcun-mount/qs-server/internal/collection-server/interface/http/middleware"
	pkgmiddleware "github.com/fangcun-mount/qs-server/internal/pkg/middleware"
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
	// 设置全局中间件
	r.setupGlobalMiddleware(engine)

	// 注册公开路由
	r.registerPublicRoutes(engine)

	// 注册API路由（collection-server不需要认证）
	r.registerAPIRoutes(engine)

	// 注册用户相关路由
	r.registerUserRoutes(engine)

	fmt.Printf("🔗 Registered routes for: public, questionnaire, answersheet, user\n")
}

// setupGlobalMiddleware 设置全局中间件
func (r *Router) setupGlobalMiddleware(engine *gin.Engine) {
	// Recovery 中间件
	engine.Use(gin.Recovery())

	// RequestID 中间件
	engine.Use(pkgmiddleware.RequestID())

	// 基础日志中间件
	engine.Use(pkgmiddleware.Logger())

	// API详细日志中间件 (可以通过配置控制是否启用)
	engine.Use(pkgmiddleware.APILogger())

	// CORS 中间件
	engine.Use(pkgmiddleware.Cors())

	// 其他中间件
	engine.Use(pkgmiddleware.NoCache)
	engine.Use(pkgmiddleware.Options)
}

// registerPublicRoutes 注册公开路由（不需要认证）
func (r *Router) registerPublicRoutes(engine *gin.Engine) {
	// 自定义健康检查路由（genericapiserver已经注册了/healthz和/version）
	engine.GET("/health", r.healthCheck)
	engine.GET("/ping", r.ping)
	engine.GET("/ready", r.readiness)
	engine.GET("/live", r.liveness)

	// 公开的API路由
	publicAPI := engine.Group("/api/v1/public")
	{
		publicAPI.GET("/info", r.getServerInfo)
		publicAPI.GET("/version", r.getVersion)
		publicAPI.GET("/config", r.getConfig)
	}
}

// registerAPIRoutes 注册API路由
func (r *Router) registerAPIRoutes(engine *gin.Engine) {
	// 创建API组
	apiV1 := engine.Group("/api/v1")

	// 注册问卷相关路由
	r.registerQuestionnaireRoutes(apiV1)

	// 注册答卷相关路由
	r.registerAnswersheetRoutes(apiV1)
}

// registerQuestionnaireRoutes 注册问卷相关路由
func (r *Router) registerQuestionnaireRoutes(apiV1 *gin.RouterGroup) {
	questionnaireHandler := r.container.QuestionnaireHandler
	if questionnaireHandler == nil {
		return
	}

	questionnaires := apiV1.Group("/questionnaires")
	{
		// 获取问卷列表和详情
		questionnaires.GET("", questionnaireHandler.List)             // 获取问卷列表
		questionnaires.GET("/:code", questionnaireHandler.Get)        // 获取问卷详情
		questionnaires.GET("/:code/raw", questionnaireHandler.GetRaw) // 获取原始问卷

		// 问卷验证（可选路由，根据需要启用）
		// questionnaires.POST("/validate", questionnaireHandler.ValidateCode)
		// questionnaires.GET("/:code/info", questionnaireHandler.GetForValidation)
	}
}

// registerAnswersheetRoutes 注册答卷相关路由
func (r *Router) registerAnswersheetRoutes(apiV1 *gin.RouterGroup) {
	answersheetHandler := r.container.AnswersheetHandler
	if answersheetHandler == nil {
		return
	}

	answersheets := apiV1.Group("/answersheets")
	{
		// 答卷核心功能
		answersheets.POST("", answersheetHandler.Submit) // 提交答卷
		answersheets.GET("/:id", answersheetHandler.Get) // 获取答卷详情
		answersheets.GET("", answersheetHandler.List)    // 获取答卷列表

		// 答卷验证（可选路由，根据需要启用）
		// answersheets.POST("/validate", answersheetHandler.Validate)
	}
}

// registerUserRoutes 注册用户相关路由
func (r *Router) registerUserRoutes(engine *gin.Engine) {
	userHandler := r.container.UserHandler
	testeeHandler := r.container.TesteeHandler
	if userHandler == nil || testeeHandler == nil {
		return
	}

	apiV1 := engine.Group("/api/v1")

	// 用户相关路由（不需要认证）
	users := apiV1.Group("/users")
	{
		// 小程序注册/登录
		users.POST("/miniprogram/register", userHandler.RegisterMiniProgram)
	}

	// 用户相关路由（需要认证）
	usersAuth := apiV1.Group("/users")
	usersAuth.Use(middleware.JWTAuth(r.container.JWTManager))
	{
		// 获取当前用户信息
		usersAuth.GET("/me", userHandler.GetUser)
	}

	// 受试者相关路由（需要认证）
	testees := apiV1.Group("/testees")
	testees.Use(middleware.JWTAuth(r.container.JWTManager))
	{
		// 创建受试者
		testees.POST("/register", testeeHandler.CreateTestee)
		// 获取当前用户的受试者信息
		testees.GET("/me", testeeHandler.GetTestee)
	}
}

// 公共路由处理函数

// getServerInfo 获取服务器信息
func (r *Router) getServerInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":      "collection-server",
		"version":      "1.0.0",
		"description":  "问卷收集服务",
		"architecture": "clean",
		"endpoints": map[string]string{
			"health":        "/health",
			"questionnaire": "/api/v1/questionnaires",
			"answersheet":   "/api/v1/answersheets",
		},
	})
}

// getVersion 获取版本信息
func (r *Router) getVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":      "1.0.0",
		"build_time":   "2024-07-21T10:30:00Z",
		"git_commit":   "latest",
		"go_version":   "go1.24.0",
		"architecture": "clean",
	})
}

// getConfig 获取配置信息
func (r *Router) getConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"cors_enabled":       true,
		"auth_enabled":       false, // collection-server不需要认证
		"logging_enabled":    true,
		"validation_enabled": true,
		"middleware": []string{
			"recovery", "request_id", "logger", "cors", "secure", "nocache", "options",
		},
	})
}

// 健康检查处理函数

// healthCheck 健康检查处理函数
func (r *Router) healthCheck(c *gin.Context) {
	response := gin.H{
		"status":       "healthy",
		"version":      "1.0.0",
		"service":      "collection-server",
		"architecture": "clean",
		"router":       "centralized",
		"auth":         "disabled", // collection-server不需要认证
		"components": gin.H{
			"grpc_clients": "questionnaire, answersheet",
			"validation":   "enabled",
			"handlers":     "questionnaire, answersheet",
			"middleware":   "enabled",
		},
	}

	c.JSON(200, response)
}

// ping 简单的连通性测试
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message":   "pong",
		"status":    "ok",
		"service":   "collection-server",
		"router":    "centralized",
		"auth":      "disabled",
		"timestamp": gin.H{"unix": 1642781200},
	})
}

// readiness 就绪检查
func (r *Router) readiness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": gin.H{
			"grpc_clients": "ready",
			"validation":   "ready",
			"handlers":     "ready",
		},
	})
}

// liveness 存活检查
func (r *Router) liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "alive",
		"service": "collection-server",
	})
}

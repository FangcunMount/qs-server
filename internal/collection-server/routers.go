package collection

import (
	"net/http"

	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
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

	// TODO: 注册业务路由
}

// setupGlobalMiddleware 设置全局中间件
func (r *Router) setupGlobalMiddleware(engine *gin.Engine) {
	// Recovery 中间件
	engine.Use(gin.Recovery())

	// RequestID 中间件
	engine.Use(pkgmiddleware.RequestID())

	// 基础日志中间件
	engine.Use(pkgmiddleware.Logger())

	// API详细日志中间件
	engine.Use(pkgmiddleware.APILogger())

	// CORS 中间件
	engine.Use(pkgmiddleware.Cors())

	// 其他中间件
	engine.Use(pkgmiddleware.NoCache)
	engine.Use(pkgmiddleware.Options)
}

// registerPublicRoutes 注册公开路由
func (r *Router) registerPublicRoutes(engine *gin.Engine) {
	// 健康检查路由
	engine.GET("/health", r.healthCheck)
	engine.GET("/ping", r.ping)

	// 公开的API路由
	publicAPI := engine.Group("/api/v1/public")
	{
		publicAPI.GET("/info", r.getServerInfo)
	}
}

// 公共路由处理函数

// getServerInfo 获取服务器信息
func (r *Router) getServerInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":     "collection-server",
		"version":     "2.0.0",
		"description": "问卷收集服务",
		"status":      "ready",
	})
}

// healthCheck 健康检查处理函数
func (r *Router) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "collection-server",
		"version": "2.0.0",
	})
}

// ping 简单的连通性测试
func (r *Router) ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
		"service": "collection-server",
	})
}

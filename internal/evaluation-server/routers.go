package evaluation

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fangcun-mount/qs-server/internal/evaluation-server/container"
	"github.com/fangcun-mount/qs-server/pkg/core"
)

// Router 路由器
type Router struct {
	container *container.Container
}

// NewRouter 创建路由器
func NewRouter(container *container.Container) *Router {
	return &Router{
		container: container,
	}
}

// RegisterRoutes 注册路由
func (r *Router) RegisterRoutes(engine *gin.Engine) {
	// 健康检查路由（仅提供健康检查，不提供业务API）
	engine.GET("/readyz", r.readinessCheck)
	engine.GET("/status", r.statusCheck)

	// 注意：evaluation-server 不对外提供 RESTful API
	// 所有业务逻辑通过消息队列触发，通过 gRPC 调用 apiserver
}

// readinessCheck 就绪检查
func (r *Router) readinessCheck(c *gin.Context) {
	// 检查容器是否已初始化
	if !r.container.IsInitialized() {
		c.JSON(http.StatusServiceUnavailable, &core.ErrResponse{
			Code:    http.StatusServiceUnavailable,
			Message: "Service is not ready",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]string{
		"status": "ready",
	})
}

// statusCheck 状态检查
func (r *Router) statusCheck(c *gin.Context) {
	status := map[string]interface{}{
		"service":     "evaluation-server",
		"version":     "1.0.0",
		"initialized": r.container.IsInitialized(),
		"description": "Questionnaire evaluation and report generation service",
	}

	c.JSON(http.StatusOK, status)
}

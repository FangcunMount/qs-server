package handler

import (
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	serviceName string
	version     string
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(serviceName, version string) *HealthHandler {
	return &HealthHandler{
		serviceName: serviceName,
		version:     version,
	}
}

// Health 健康检查
// @Summary 健康检查
// @Description 检查服务健康状态
// @Tags 系统
// @Produce json
// @Success 200 {object} core.Response
// @Router /health [get]
func (h *HealthHandler) Health(c *gin.Context) {
	core.WriteResponse(c, nil, gin.H{
		"status":  "healthy",
		"service": h.serviceName,
		"version": h.version,
	})
}

// Ping 简单连通性测试
// @Summary Ping
// @Description 测试服务连通性
// @Tags 系统
// @Produce json
// @Success 200 {object} core.Response
// @Router /ping [get]
func (h *HealthHandler) Ping(c *gin.Context) {
	core.WriteResponse(c, nil, gin.H{
		"message": "pong",
		"service": h.serviceName,
	})
}

// Info 服务信息
// @Summary 服务信息
// @Description 获取服务基本信息
// @Tags 系统
// @Produce json
// @Success 200 {object} core.Response
// @Router /api/v1/public/info [get]
func (h *HealthHandler) Info(c *gin.Context) {
	core.WriteResponse(c, nil, gin.H{
		"service":     h.serviceName,
		"version":     h.version,
		"description": "问卷收集服务 - BFF 层",
		"status":      "ready",
	})
}

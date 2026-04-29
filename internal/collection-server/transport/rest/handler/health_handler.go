package handler

import (
	"net/http"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	serviceName string
	version     string
	status      *observability.FamilyStatusRegistry
	resilience  func() resilienceplane.RuntimeSnapshot
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(serviceName, version string, status *observability.FamilyStatusRegistry) *HealthHandler {
	return NewHealthHandlerWithResilience(serviceName, version, status, nil)
}

func NewHealthHandlerWithResilience(serviceName, version string, status *observability.FamilyStatusRegistry, resilience func() resilienceplane.RuntimeSnapshot) *HealthHandler {
	return &HealthHandler{
		serviceName: serviceName,
		version:     version,
		status:      status,
		resilience:  resilience,
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
		"redis":   observability.SnapshotForComponent(h.serviceName, h.status),
	})
}

// Ready 就绪检查
func (h *HealthHandler) Ready(c *gin.Context) {
	snapshot := observability.SnapshotForComponent(h.serviceName, h.status)
	statusCode := http.StatusOK
	statusText := "ready"
	if !snapshot.Summary.Ready {
		statusCode = http.StatusServiceUnavailable
		statusText = "degraded"
	}
	c.JSON(statusCode, core.Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"status":  statusText,
			"service": h.serviceName,
			"version": h.version,
			"redis":   snapshot,
		},
	})
}

// RedisFamilies 返回 Redis family 治理快照
func (h *HealthHandler) RedisFamilies(c *gin.Context) {
	core.WriteResponse(c, nil, observability.SnapshotForComponent(h.serviceName, h.status))
}

// Resilience 返回 collection-server 高并发治理只读快照。
func (h *HealthHandler) Resilience(c *gin.Context) {
	if h != nil && h.resilience != nil {
		core.WriteResponse(c, nil, h.resilience())
		return
	}
	component := "collection-server"
	if h != nil && h.serviceName != "" {
		component = h.serviceName
	}
	core.WriteResponse(c, nil, resilienceplane.FinalizeRuntimeSnapshot(resilienceplane.NewRuntimeSnapshot(component, time.Now())))
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
		"redis":       observability.SnapshotForComponent(h.serviceName, h.status),
	})
}

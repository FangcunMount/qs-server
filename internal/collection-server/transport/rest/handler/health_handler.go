package handler

import (
	"net/http"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	serviceName  string
	version      string
	status       *observability.FamilyStatusRegistry
	resilience   func() resilience.RuntimeSnapshot
	controlReady func() bool
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(serviceName, version string, status *observability.FamilyStatusRegistry) *HealthHandler {
	return NewHealthHandlerWithResilience(serviceName, version, status, nil)
}

func NewHealthHandlerWithResilience(serviceName, version string, status *observability.FamilyStatusRegistry, resilience func() resilience.RuntimeSnapshot, readiness ...func() bool) *HealthHandler {
	handler := &HealthHandler{
		serviceName: serviceName,
		version:     version,
		status:      status,
		resilience:  resilience,
	}
	if len(readiness) > 0 {
		handler.controlReady = readiness[0]
	}
	return handler
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
		"redis":   h.redisSnapshot(),
	})
}

// Ready 就绪检查
// @Summary 就绪检查
// @Description 返回 collection-server 及 Redis 依赖的就绪状态。
// @Tags 系统
// @Produce json
// @Success 200 {object} core.Response
// @Failure 503 {object} core.Response
// @Router /readyz [get]
func (h *HealthHandler) Ready(c *gin.Context) {
	snapshot, controlSynchronized := h.readiness()
	statusCode := http.StatusOK
	statusText := "ready"
	if !snapshot.Summary.Ready {
		statusCode = http.StatusServiceUnavailable
		statusText = "degraded"
	}
	if !controlSynchronized {
		statusCode = http.StatusServiceUnavailable
		statusText = "synchronizing"
	}
	c.JSON(statusCode, core.Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"status":                          statusText,
			"service":                         h.serviceName,
			"version":                         h.version,
			"redis":                           snapshot,
			"resilience_control_synchronized": controlSynchronized,
		},
	})
}

// ServeReady reports whether the process may continue serving low traffic.
// Redis dependency degradation remains visible in the response, while the
// initial resilience-control synchronization is still a hard readiness gate.
//
// @Summary 服务就绪检查
// @Description 首次韧性控制同步后，即使 Redis family 降级也允许继续承接低流量。
// @Tags 系统
// @Produce json
// @Success 200 {object} core.Response
// @Failure 503 {object} core.Response
// @Router /serve-readyz [get]
func (h *HealthHandler) ServeReady(c *gin.Context) {
	snapshot, controlSynchronized := h.readiness()
	dependencyReady := snapshot.Summary.Ready
	serveReady := controlSynchronized
	statusCode := http.StatusOK
	statusText := "ready"
	if !dependencyReady {
		statusText = "degraded"
	}
	if !serveReady {
		statusCode = http.StatusServiceUnavailable
		statusText = "synchronizing"
	}
	c.JSON(statusCode, core.Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"status":                          statusText,
			"service":                         h.serviceName,
			"version":                         h.version,
			"serve_ready":                     serveReady,
			"dependency_ready":                dependencyReady,
			"redis":                           snapshot,
			"resilience_control_synchronized": controlSynchronized,
		},
	})
}

func (h *HealthHandler) readiness() (observability.RuntimeSnapshot, bool) {
	snapshot := h.redisSnapshot()
	controlSynchronized := h.controlReady == nil || h.controlReady()
	return snapshot, controlSynchronized
}

func (h *HealthHandler) redisSnapshot() observability.RuntimeSnapshot {
	if h == nil {
		return observability.RuntimeSnapshot{}
	}
	snapshot := observability.SnapshotForComponent(h.serviceName, h.status)
	if h.resilience != nil {
		identity := h.resilience()
		snapshot.InstanceID = identity.InstanceID
		snapshot.Generation = identity.Generation
	}
	return snapshot
}

// RedisFamilies 返回 Redis family 治理快照
// @Summary Redis 治理状态
// @Description 返回 collection-server Redis family 的运行状态。
// @Tags 系统
// @Produce json
// @Success 200 {object} core.Response
// @Router /governance/redis [get]
func (h *HealthHandler) RedisFamilies(c *gin.Context) {
	core.WriteResponse(c, nil, h.redisSnapshot())
}

// Resilience 返回 collection-server 高并发治理只读快照。
// @Summary 韧性治理状态
// @Description 返回限流、并发控制与降级能力的运行快照。
// @Tags 系统
// @Produce json
// @Success 200 {object} core.Response
// @Router /governance/resilience [get]
func (h *HealthHandler) Resilience(c *gin.Context) {
	if h != nil && h.resilience != nil {
		core.WriteResponse(c, nil, h.resilience())
		return
	}
	component := "collection-server"
	if h != nil && h.serviceName != "" {
		component = h.serviceName
	}
	core.WriteResponse(c, nil, resilience.FinalizeRuntimeSnapshot(resilience.NewRuntimeSnapshot(component, time.Now())))
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
		"redis":       h.redisSnapshot(),
	})
}

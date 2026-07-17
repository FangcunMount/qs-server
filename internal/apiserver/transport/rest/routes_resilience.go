package rest

import (
	"net/http"
	"time"

	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerResilienceInternalRoutes(internalV1 *gin.RouterGroup) {
	resilience := internalV1.Group("/resilience", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	resilience.GET("/status", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		r.resilienceStatus,
	)...)
}

// resilienceStatus returns the internal runtime resilience snapshot.
// @Summary 韧性治理状态
// @Description 返回限流、背压和锁的运行时治理快照，仅内部管理员可访问。
// @Tags System-Governance
// @Produce json
// @Success 200 {object} resilienceplane.RuntimeSnapshot
// @Router /internal/v1/resilience/status [get]
func (r *Router) resilienceStatus(c *gin.Context) {
	c.JSON(http.StatusOK, r.resilienceSnapshot())
}

func (r *Router) resilienceSnapshot() resilienceplane.RuntimeSnapshot {
	snapshot := resilienceplane.NewRuntimeSnapshot("apiserver", time.Now())
	rateEnabled := r != nil && r.rateCfg != nil && r.rateCfg.Enabled
	snapshot.RateLimits = []resilienceplane.CapabilitySnapshot{
		{Name: "rest_global", Kind: resilienceplane.ProtectionRateLimit.String(), Strategy: "local", Configured: rateEnabled},
		{Name: "rest_user", Kind: resilienceplane.ProtectionRateLimit.String(), Strategy: "local_key", Configured: rateEnabled},
	}
	if r != nil {
		snapshot.Backpressure = append(snapshot.Backpressure, r.deps.Backpressure...)
	}
	if r != nil {
		snapshot.Locks = append(snapshot.Locks, r.deps.Locks...)
	}
	return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
}

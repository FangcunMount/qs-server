package rest

import (
	"net/http"
	"time"

	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
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
	snapshot.Locks = []resilienceplane.CapabilitySnapshot{
		{Name: "plan_scheduler_leader", Kind: resilienceplane.ProtectionLock.String(), Strategy: "redis_lock", Configured: true},
		{Name: "statistics_sync_leader", Kind: resilienceplane.ProtectionLock.String(), Strategy: "redis_lock", Configured: true},
		{Name: "behavior_pending_reconcile", Kind: resilienceplane.ProtectionLock.String(), Strategy: "redis_lock", Configured: true},
	}
	return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
}

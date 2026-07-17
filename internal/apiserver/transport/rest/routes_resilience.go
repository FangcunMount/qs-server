package rest

import (
	"net/http"

	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerResilienceInternalRoutes(internalV1 *gin.RouterGroup) {
	resilienceGroup := internalV1.Group("/resilience", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	resilienceGroup.GET("/status", r.rateLimitedHandlers(rateLimitBudgetQuery, r.resilienceStatus)...)
}

// resilienceStatus returns the internal runtime resilience snapshot.
// @Summary 韧性治理状态
// @Description 返回限流、背压和锁的运行时治理快照，仅内部管理员可访问。
// @Tags System-Governance
// @Produce json
// @Success 200 {object} resilience.RuntimeSnapshot
// @Router /internal/v1/resilience/status [get]
func (r *Router) resilienceStatus(c *gin.Context) {
	c.JSON(http.StatusOK, r.resilienceSnapshot())
}

func (r *Router) resilienceSnapshot() resilience.RuntimeSnapshot {
	if r != nil && r.deps.ResilienceSnapshot != nil {
		return r.deps.ResilienceSnapshot()
	}
	return resilience.RuntimeSnapshot{Component: "apiserver"}
}

package rest

import (
	"net/http"

	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerEventStatusInternalRoutes(internalV1 *gin.RouterGroup) {
	if r.deps.EventStatusService == nil {
		return
	}

	events := internalV1.Group("/events", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	events.GET("/status", r.rateLimitedHandlers(
		r.rateCfg,
		r.rateCfg.QueryGlobalQPS,
		r.rateCfg.QueryGlobalBurst,
		r.rateCfg.QueryUserQPS,
		r.rateCfg.QueryUserBurst,
		r.eventStatus,
	)...)
}

func (r *Router) eventStatus(c *gin.Context) {
	snapshot, err := r.deps.EventStatusService.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

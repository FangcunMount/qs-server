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
	events.GET("/status", r.rateLimitedHandlers(rateLimitBudgetQuery, r.eventStatus)...)
}

// eventStatus returns the event delivery status.
// @Summary 事件投递状态
// @Description 返回事件与 outbox 的只读运行快照，仅内部管理员可访问。
// @Tags System-Governance
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /internal/v1/events/status [get]
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

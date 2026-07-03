package handler

import (
	"net/http"

	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/gin-gonic/gin"
)

// SystemGovernanceHandler serves unified governance endpoints.
type SystemGovernanceHandler struct {
	BaseHandler
	facade systemgov.Facade
}

// NewSystemGovernanceHandler creates a governance handler.
func NewSystemGovernanceHandler(facade systemgov.Facade) *SystemGovernanceHandler {
	return &SystemGovernanceHandler{
		BaseHandler: *NewBaseHandler(),
		facade:      facade,
	}
}

func (h *SystemGovernanceHandler) Overview(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	result, err := h.facade.GetOverview(c.Request.Context(), c.Query("window"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

func (h *SystemGovernanceHandler) Events(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	result, err := h.facade.GetEvents(c.Request.Context(), c.Query("window"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

func (h *SystemGovernanceHandler) Cache(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	result, err := h.facade.GetCache(c.Request.Context(), c.Query("window"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

func (h *SystemGovernanceHandler) Resilience(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	result, err := h.facade.GetResilience(c.Request.Context(), c.Query("window"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

func (h *SystemGovernanceHandler) Actions(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	result, err := h.facade.ListActions(c.Request.Context())
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

func (h *SystemGovernanceHandler) RunAction(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var req systemgov.ActionRunRequest
	if !h.bindJSON(c, &req) {
		return
	}
	result, err := h.facade.RunAction(c.Request.Context(), orgID, c.Param("action_id"), req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

func (h *SystemGovernanceHandler) bindJSON(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		h.Error(c, err)
		return false
	}
	return true
}

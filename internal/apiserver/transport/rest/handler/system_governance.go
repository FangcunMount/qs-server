package handler

import (
	"net/http"
	"strconv"

	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/gin-gonic/gin"
)

// SystemGovernanceHandler serves unified governance endpoints.
type SystemGovernanceHandler struct {
	BaseHandler
	facade systemgov.Facade
}

// RetryCandidates returns an organization-scoped, bounded governance worklist.
// @Summary 系统治理-重试候选
// @Description 返回当前组织可治理的最新业务失败、Outbox 人工重放项与运输死信；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param cursor query string false "不透明分页游标"
// @Param limit query int false "每页条数，1-100" default(50)
// @Success 200 {object} core.Response{data=systemgovernance.RetryCandidatePage}
// @Failure 400 {object} core.ErrResponse
// @Router /internal/v1/system-governance/events/retry-candidates [get]
func (h *SystemGovernanceHandler) RetryCandidates(c *gin.Context) {
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "limit must be between 1 and 100"})
			return
		}
		limit = parsed
	}
	result, err := h.facade.ListRetryCandidates(c.Request.Context(), orgID, c.Query("cursor"), limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// NewSystemGovernanceHandler creates a governance handler.
func NewSystemGovernanceHandler(facade systemgov.Facade) *SystemGovernanceHandler {
	return &SystemGovernanceHandler{
		BaseHandler: *NewBaseHandler(),
		facade:      facade,
	}
}

// Overview returns the unified governance workbench snapshot.
// @Summary 系统治理总览
// @Description 聚合事件、缓存、承压保护诊断信号与近窗口指标可用性；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param window query string false "指标窗口，如 5m、15m、1h" default(5m)
// @Success 200 {object} core.Response{data=systemgovernance.OverviewResponse}
// @Failure 400 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/system-governance/overview [get]
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

// Events returns event/outbox governance detail.
// @Summary 系统治理-事件排水
// @Description 返回 outbox 快照、event_type 维度积压与诊断信号；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param window query string false "指标窗口，如 5m、15m、1h" default(5m)
// @Success 200 {object} core.Response
// @Failure 400 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/system-governance/events [get]
func (h *SystemGovernanceHandler) Events(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.facade.GetEvents(c.Request.Context(), orgID, c.Query("window"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// Cache returns cache governance detail.
// @Summary 系统治理-缓存预热
// @Description 返回缓存 runtime/warmup 快照与诊断信号；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param window query string false "指标窗口，如 5m、15m、1h" default(5m)
// @Success 200 {object} core.Response{data=systemgovernance.CacheView}
// @Failure 400 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/system-governance/cache [get]
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

// Resilience returns aggregated resilience governance detail.
// @Summary 系统治理-承压保护
// @Description 聚合 apiserver、collection-server、worker 韧性快照与诊断信号；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param window query string false "指标窗口，如 5m、15m、1h" default(5m)
// @Success 200 {object} core.Response{data=systemgovernance.ResilienceView}
// @Failure 400 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/system-governance/resilience [get]
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

// Actions lists governance command descriptors.
// @Summary 系统治理-动作目录
// @Description 返回可执行与预留治理动作描述符；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Success 200 {object} core.Response{data=systemgovernance.ActionsView}
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions [get]
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

// RunAction executes one enabled governance command.
// @Summary 执行治理动作
// @Description 执行已启用的缓存或韧性治理动作；韧性动作默认关闭，要求 confirm=true，并可通过 request_id 安全重试；仅 qs:admin 可访问
// @Tags System-Governance
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param action_id path string true "动作 ID"
// @Param request body systemgovernance.ActionRunRequest true "动作参数"
// @Success 200 {object} core.Response{data=systemgovernance.ActionRunResult}
// @Failure 400 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/{action_id}/runs [post]
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

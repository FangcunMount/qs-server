package handler

import (
	"net/http"

	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/gin-gonic/gin"
)

// GetGapRecoverySummary reads the host review ledger independently of the
// generic Outbox manual-replay counters. It remains available when new
// recovery authorization is switched off.
// @Summary 系统治理-原测评消息恢复审核汇总
// @Description 按当前机构读取持久批准／拒绝总数和仍待 Relay 的原消息数；仅 qs:admin 可访问。
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Success 200 {object} core.Response{data=systemgovernance.GapRecoverySummary}
// @Router /internal/v1/system-governance/actions/gap-recoveries/summary [get]
func (h *SystemGovernanceHandler) GetGapRecoverySummary(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	orgID, _, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.facade.GetGapRecoverySummary(c.Request.Context(), orgID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// AuthorizeGapRecovery records one reviewed recovery of the original event.
// It cannot generate a new event or send to NSQ directly.
// @Summary 系统治理-授权恢复未接单的原测评消息
// @Description 仅在显式启用时可用；同请求编号与完整输入可安全重试，结果未知先查询原请求。仅 qs:admin 可访问。
// @Tags System-Governance
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body systemgovernance.GapRecoveryRequest true "原事件身份、预期版本、宽限截点、理由及确认"
// @Success 200 {object} core.Response{data=systemgovernance.GapRecoveryDecision}
// @Failure 400 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/gap-recoveries [post]
func (h *SystemGovernanceHandler) AuthorizeGapRecovery(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	orgID, actorID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var req systemgov.GapRecoveryRequest
	if !h.bindJSON(c, &req) {
		return
	}
	result, err := h.facade.AuthorizeGapRecovery(c.Request.Context(), orgID, uint64(actorID), req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// ResolveGapRecovery reads the decision for the original actor and exact input.
// A missing request is not permission to create a new recovery request.
// @Summary 系统治理-核对原测评消息恢复决定
// @Description 只读核对同机构、同操作者、同请求编号与输入的持久决定；仅 qs:admin 可访问。
// @Tags System-Governance
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body systemgovernance.GapRecoveryRequest true "原操作的完整输入"
// @Success 200 {object} core.Response{data=systemgovernance.GapRecoveryDecision}
// @Failure 404 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/gap-recoveries/resolve [post]
func (h *SystemGovernanceHandler) ResolveGapRecovery(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	orgID, actorID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var req systemgov.GapRecoveryRequest
	if !h.bindJSON(c, &req) {
		return
	}
	result, found, err := h.facade.ResolveGapRecovery(c.Request.Context(), orgID, uint64(actorID), req)
	if err != nil {
		h.Error(c, err)
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"message": "gap recovery request not found"})
		return
	}
	h.Success(c, result)
}

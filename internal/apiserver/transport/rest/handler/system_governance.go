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

// PendingReplayAudits lists unresolved replay requests without suggesting a
// new authorization. The original actor must retry the same ID and input.
// @Summary 系统治理-待核对重放操作
// @Description 按当前组织列出结果未知的人工重放审批；仅 qs:admin 可访问。核对时须由原操作者沿用原请求编号与输入。
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param cursor query string false "上一页返回的数字 ID 游标"
// @Param limit query int false "每页条数，1-100" default(50)
// @Success 200 {object} core.Response{data=systemgovernance.PendingReplayAuditPage}
// @Failure 400 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/pending-reconciliations [get]
func (h *SystemGovernanceHandler) PendingReplayAudits(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
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
	cursor := c.Query("cursor")
	if cursor != "" {
		parsed, parseErr := strconv.ParseUint(cursor, 10, 64)
		if parseErr != nil || parsed == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid cursor"})
			return
		}
	}
	result, err := h.facade.ListPendingReplayAudits(c.Request.Context(), orgID, cursor, limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// DeliveryReplayReviews lists old unfinished audits and failed audits with
// uncertain transport delivery. The response never authorizes another send.
// @Summary 系统治理-待核对的传输重放审计
// @Description 列出当前机构超过五分钟仍未结案的审计，以及失败或超时但仍有关联待核对死信的传输重放审计；只读，不重新投递消息；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param cursor query string false "分页游标"
// @Param limit query int false "每页条数，1-100" default(50)
// @Success 200 {object} core.Response{data=systemgovernance.DeliveryReplayReviewPage}
// @Failure 400 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/delivery-replay-reviews [get]
func (h *SystemGovernanceHandler) DeliveryReplayReviews(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
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
	cursor := c.Query("cursor")
	if cursor != "" {
		parsed, parseErr := strconv.ParseUint(cursor, 10, 64)
		if parseErr != nil || parsed == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid cursor"})
			return
		}
	}
	result, err := h.facade.ListDeliveryReplayReviews(c.Request.Context(), orgID, cursor, limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// ReminderReviews lists uncertain task-opened reminder sends for manual
// reconciliation. This endpoint cannot authorize or trigger another send.
// @Summary 系统治理-待核对的任务开放提醒
// @Description 按当前机构列出发送结果未知的任务开放提醒；只读，不自动补发；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param cursor query string false "不透明分页游标"
// @Param limit query int false "每页条数，1-100" default(50)
// @Success 200 {object} core.Response{data=systemgovernance.ReminderReviewPage}
// @Failure 400 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/reminder-reviews [get]
func (h *SystemGovernanceHandler) ReminderReviews(c *gin.Context) {
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
	result, err := h.facade.ListReminderReviews(c.Request.Context(), orgID, c.Query("cursor"), limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// DeliveryResolutionHTTPRequest is the explicit operator command body. The
// organization and actor are always taken from the protected request context.
type DeliveryResolutionHTTPRequest struct {
	RequestID                string `json:"request_id"`
	OriginalReplayRequestID  string `json:"original_replay_request_id"`
	DeadLetterID             uint64 `json:"dead_letter_id"`
	EventID                  string `json:"event_id"`
	ExpectedDeliveryAttempts int    `json:"expected_delivery_attempts"`
	Reason                   string `json:"reason"`
	Confirm                  bool   `json:"confirm"`
}

// ResolveDelivery records a separate, fact-verified resolution without
// republishing. Its audit and dead-letter update share one MySQL transaction.
// @Summary 系统治理-核实传输死信的业务结果后结案
// @Description 仅对具备完整业务事实验证器的报告生成事件结案；不投递消息；仅 qs:admin 可访问
// @Tags System-Governance
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param request body DeliveryResolutionHTTPRequest true "原操作与物理死信身份、稳定请求编号及确认"
// @Success 200 {object} core.Response{data=systemgovernance.ActionRunResult}
// @Failure 400 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/delivery-resolutions [post]
func (h *SystemGovernanceHandler) ResolveDelivery(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	orgID, actorID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	var body DeliveryResolutionHTTPRequest
	if !h.bindJSON(c, &body) {
		return
	}
	req := systemgov.DeliveryResolutionRequest{
		RequestID: body.RequestID, OriginalReplayRequestID: body.OriginalReplayRequestID,
		DeadLetterID: body.DeadLetterID, EventID: body.EventID,
		ExpectedDeliveryAttempts: body.ExpectedDeliveryAttempts, Reason: body.Reason,
		Confirm: body.Confirm,
	}
	result, err := h.facade.ResolveDelivery(c.Request.Context(), orgID, uint64(actorID), req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// GetDeliveryResolution retrieves a committed, organization-scoped receipt.
// @Summary 系统治理-查询已核实传输死信结案回执
// @Description 查询同机构已提交的结案审计；只读，不重新核验或投递；仅 qs:admin 可访问
// @Tags System-Governance
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌（或内部调用token）"
// @Param request_id path string true "结案请求编号"
// @Success 200 {object} core.Response{data=systemgovernance.ActionRunResult}
// @Failure 404 {object} core.ErrResponse
// @Router /internal/v1/system-governance/actions/delivery-resolutions/{request_id} [get]
func (h *SystemGovernanceHandler) GetDeliveryResolution(c *gin.Context) {
	if h.facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "system governance unavailable"})
		return
	}
	orgID, err := h.RequireProtectedOrgID(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.facade.GetDeliveryResolution(c.Request.Context(), orgID, c.Param("request_id"))
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

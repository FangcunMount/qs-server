package handler

import (
	"errors"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"strconv"
)

type AIWorkflowPromptDraftHandler struct {
	*BaseHandler
	service *app.PromptDraftAdministration
}

func NewAIWorkflowPromptDraftHandler(s *app.PromptDraftAdministration) *AIWorkflowPromptDraftHandler {
	return &AIWorkflowPromptDraftHandler{NewBaseHandler(), s}
}
func (h *AIWorkflowPromptDraftHandler) scope(c *gin.Context) (app.DraftScope, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.DraftScope{}, false
	}
	return app.DraftScope{OrganizationID: org, OperatorUserID: user}, true
}
func (h *AIWorkflowPromptDraftHandler) failure(c *gin.Context, err error) {
	errorCode, message := code.ErrUnknown, "AI draft outcome unknown; query the original command ID before saving again"
	switch {
	case errors.Is(err, app.ErrInvalid), status.Code(err) == codes.InvalidArgument:
		errorCode, message = code.ErrInvalidArgument, "Invalid AI draft operation"
	case errors.Is(err, app.ErrGovernanceDenied), status.Code(err) == codes.PermissionDenied:
		errorCode, message = code.ErrPermissionDenied, "AI draft governance permission required"
	case errors.Is(err, app.ErrNotFound), status.Code(err) == codes.NotFound:
		errorCode, message = code.ErrPageNotFound, "AI draft revision or command receipt unavailable"
	case errors.Is(err, app.ErrConflict), status.Code(err) == codes.Aborted:
		errorCode, message = code.ErrConflict, "AI draft version or confirmed state changed"
	case errors.Is(err, app.ErrManagementUnavailable):
		errorCode, message = code.ErrUnsupportedOperation, "AI draft management disabled"
	}
	h.Error(c, cberrors.WithCode(errorCode, "%s", message))
}

// Create godoc
// @Summary 从现有资产创建 qs-ai Prompt 草稿
// @Description 需要当前机构 OrgAdmin 权限；组织和操作人取认证上下文。保存不代表校验或发布。超时后查询原 command_id，不自动重试。
// @Tags AI-Workflow-Prompt-Drafts
// @Accept json
// @Produce json
// @Param draft_id path string true "草稿 UUID"
// @Param body body app.CreatePromptDraft true "保存命令"
// @Success 200 {object} core.Response{data=app.PromptDraftState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/prompt-drafts/{draft_id}/create [post]
func (h *AIWorkflowPromptDraftHandler) Create(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	var command app.CreatePromptDraft
	if err := c.ShouldBindJSON(&command); err != nil {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.Create(c.Request.Context(), scope, c.Param("draft_id"), command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Revise godoc
// @Summary 保存 qs-ai Prompt 草稿新修订
// @Description 需要当前机构 OrgAdmin 权限；组织和操作人取认证上下文。保存不代表校验或发布。超时后查询原 command_id，不自动重试。
// @Tags AI-Workflow-Prompt-Drafts
// @Accept json
// @Produce json
// @Param draft_id path string true "草稿 UUID"
// @Param body body app.RevisePromptDraft true "保存命令"
// @Success 200 {object} core.Response{data=app.PromptDraftState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/prompt-drafts/{draft_id}/revisions [post]
func (h *AIWorkflowPromptDraftHandler) Revise(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	var command app.RevisePromptDraft
	if err := c.ShouldBindJSON(&command); err != nil {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.Revise(c.Request.Context(), scope, c.Param("draft_id"), command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Get godoc
// @Summary 读取 qs-ai Prompt 草稿当前或历史修订
// @Description 需要当前机构解读审计权限。查询不触发校验、发布或模型调用。
// @Tags AI-Workflow-Prompt-Drafts
// @Produce json
// @Param draft_id path string true "草稿 UUID"
// @Param revision query int false "正整数修订号；省略读取当前版本"
// @Success 200 {object} core.Response{data=app.PromptDraftState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/prompt-drafts/{draft_id} [get]
func (h *AIWorkflowPromptDraftHandler) Get(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var revision *int64
	if raw, exists := c.Request.URL.Query()["revision"]; exists {
		if len(raw) != 1 {
			h.failure(c, app.ErrInvalid)
			return
		}
		value, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil || value < 1 {
			h.failure(c, app.ErrInvalid)
			return
		}
		revision = &value
	}
	value, err := h.service.Get(c.Request.Context(), scope, c.Param("draft_id"), revision)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// GetReceipt godoc
// @Summary 查询 qs-ai 草稿保存的原始命令回执
// @Description 需要当前机构解读审计权限，限原组织及原操作人。仅查询，不重发保存命令。
// @Tags AI-Workflow-Prompt-Drafts
// @Produce json
// @Param command_id path string true "原命令 UUID"
// @Success 200 {object} core.Response{data=app.PromptDraftState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/prompt-drafts/commands/{command_id} [get]
func (h *AIWorkflowPromptDraftHandler) GetReceipt(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	value, err := h.service.GetReceipt(c.Request.Context(), scope, c.Param("command_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Freeze godoc
// @Summary 校验语法并冻结 qs-ai 原生 Prompt 资产
// @Description 需要当前机构 OrgAdmin 权限；组织和操作人取认证上下文。冻结只校验模板语法，不代表质量评测批准或发布。超时后查询原 command_id，不自动重试。
// @Tags AI-Workflow-Prompt-Drafts
// @Accept json
// @Produce json
// @Param draft_id path string true "草稿 UUID"
// @Param body body app.FreezePromptDraft true "保存命令"
// @Success 200 {object} core.Response{data=app.FrozenPromptReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/prompt-drafts/{draft_id}/freeze [post]
func (h *AIWorkflowPromptDraftHandler) Freeze(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	var command app.FreezePromptDraft
	if err := c.ShouldBindJSON(&command); err != nil {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.Freeze(c.Request.Context(), scope, c.Param("draft_id"), command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// GetFreezeReceipt godoc
// @Summary 查询 qs-ai Prompt 冻结的原始命令回执
// @Description 需要当前机构解读审计权限，限原组织及原操作人。仅查询，不重发保存命令。
// @Tags AI-Workflow-Prompt-Drafts
// @Produce json
// @Param command_id path string true "原命令 UUID"
// @Success 200 {object} core.Response{data=app.FrozenPromptReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/prompt-drafts/freeze-commands/{command_id} [get]
func (h *AIWorkflowPromptDraftHandler) GetFreezeReceipt(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	value, err := h.service.GetFreezeReceipt(c.Request.Context(), scope, c.Param("command_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

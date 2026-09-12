package handler

import (
	"errors"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AIWorkflowManagementHandler struct {
	*BaseHandler
	service *app.EvaluationAdministration
}

func NewAIWorkflowManagementHandler(service *app.EvaluationAdministration) *AIWorkflowManagementHandler {
	return &AIWorkflowManagementHandler{BaseHandler: NewBaseHandler(), service: service}
}
func (h *AIWorkflowManagementHandler) scope(c *gin.Context) (app.EvaluationScope, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.EvaluationScope{}, false
	}
	return app.EvaluationScope{RunID: c.Param("run_id"), OrganizationID: org, OperatorUserID: user}, true
}
func (h *AIWorkflowManagementHandler) failure(c *gin.Context, err error) {
	errorCode, message := code.ErrUnknown, "AI evaluation outcome unknown; read current state before deciding again"
	switch {
	case errors.Is(err, app.ErrInvalid), status.Code(err) == codes.InvalidArgument:
		errorCode, message = code.ErrInvalidArgument, "Invalid AI evaluation operation"
	case errors.Is(err, app.ErrGovernanceDenied), status.Code(err) == codes.PermissionDenied:
		errorCode, message = code.ErrPermissionDenied, "AI evaluation governance permission required"
	case errors.Is(err, app.ErrNotFound), status.Code(err) == codes.NotFound:
		errorCode, message = code.ErrPageNotFound, "AI evaluation unavailable"
	case errors.Is(err, app.ErrConflict), status.Code(err) == codes.Aborted:
		errorCode, message = code.ErrConflict, "AI evaluation version or state changed"
	case errors.Is(err, app.ErrManagementUnavailable):
		errorCode, message = code.ErrUnsupportedOperation, "AI evaluation management disabled"
	}
	h.Error(c, cberrors.WithCode(errorCode, "%s", message))
}

// Get godoc
// @Summary 查询 qs-ai 评测状态
// @Description 需要当前机构解读审计权限；组织和操作人取认证上下文。管理功能默认关闭。
// @Tags AI-Workflow-Management
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Success 200 {object} core.Response{data=app.EvaluationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id} [get]
func (h *AIWorkflowManagementHandler) Get(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	value, err := h.service.Get(c.Request.Context(), scope)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// ResolveUnknown godoc
// @Summary 处置 qs-ai 评测未知结果
// @Description 需要当前机构 OrgAdmin 权限；组织和操作人取认证上下文。管理功能默认关闭。
// @Tags AI-Workflow-Management
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Accept json
// @Description 需确认重复调用和费用风险；超时后先查询状态，不自动重试。
// @Param body body app.UnknownResolution true "人工决定及预期版本"
// @Success 200 {object} core.Response{data=app.EvaluationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/result-unknown/resolve [post]
func (h *AIWorkflowManagementHandler) ResolveUnknown(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.UnknownResolution
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Resolve(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Start godoc
// @Summary 启动已冻结的 qs-ai 评测
// @Description 需要当前机构 OrgAdmin 权限、明确确认和当前版本；超时后先回读状态。管理功能默认关闭。
// @Tags AI-Workflow-Management
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param body body app.EvaluationStart true "启动确认和预期版本"
// @Success 200 {object} core.Response{data=app.EvaluationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/start [post]
func (h *AIWorkflowManagementHandler) Start(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.EvaluationStart
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Start(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Create godoc
// @Summary 创建冻结的 qs-ai 评测
// @Description 需要当前机构 OrgAdmin 权限、明确确认和固定 Run UUID；同一请求重放复用原 Run，内容冲突拒绝。管理功能默认关闭。
// @Tags AI-Workflow-Management
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param body body app.EvaluationCreate true "冻结引用和创建确认"
// @Success 200 {object} core.Response{data=app.EvaluationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/create [post]
func (h *AIWorkflowManagementHandler) Create(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.EvaluationCreate
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Create(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Review godoc
// @Summary 批量审核 qs-ai 评测候选
// @Description 需要当前机构 OrgAdmin 权限；每批最多 35 个候选和一种审核职责。身份来自认证上下文，审核时间由 AI 服务端记录。超时后先回读状态，不自动重试。管理功能默认关闭。
// @Tags AI-Workflow-Management
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run UUID"
// @Param body body app.EvaluationReview true "审核职责、候选决定和预期版本"
// @Success 200 {object} core.Response{data=app.EvaluationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/evaluations/{run_id}/reviews [post]
func (h *AIWorkflowManagementHandler) Review(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.EvaluationReview
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Review(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

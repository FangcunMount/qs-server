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

type AIWorkflowPublicationHandler struct {
	*BaseHandler
	service *app.PublicationAdministration
}

func NewAIWorkflowPublicationHandler(service *app.PublicationAdministration) *AIWorkflowPublicationHandler {
	return &AIWorkflowPublicationHandler{BaseHandler: NewBaseHandler(), service: service}
}
func (h *AIWorkflowPublicationHandler) scope(c *gin.Context) (app.PublicationScope, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.PublicationScope{}, false
	}
	return app.PublicationScope{OrganizationID: org, OperatorUserID: user}, true
}
func (h *AIWorkflowPublicationHandler) failure(c *gin.Context, err error) {
	errorCode, message := code.ErrUnknown, "AI publication outcome unknown; query the original command ID before deciding again"
	switch {
	case errors.Is(err, app.ErrInvalid), status.Code(err) == codes.InvalidArgument:
		errorCode, message = code.ErrInvalidArgument, "Invalid AI publication operation"
	case errors.Is(err, app.ErrGovernanceDenied), status.Code(err) == codes.PermissionDenied:
		errorCode, message = code.ErrPermissionDenied, "AI publication governance permission required"
	case errors.Is(err, app.ErrNotFound), status.Code(err) == codes.NotFound:
		errorCode, message = code.ErrPageNotFound, "AI publication or command receipt unavailable"
	case errors.Is(err, app.ErrConflict), status.Code(err) == codes.Aborted:
		errorCode, message = code.ErrConflict, "AI publication version or confirmed state changed"
	case errors.Is(err, app.ErrManagementUnavailable):
		errorCode, message = code.ErrUnsupportedOperation, "AI publication management disabled"
	}
	h.Error(c, cberrors.WithCode(errorCode, "%s", message))
}

// Publish godoc
// @Summary 发布通过评测的 qs-ai 配置
// @Description 需要当前机构 OrgAdmin 权限；组织和操作人取认证上下文。必须确认预期选择器版本及当前发布 ID；超时后按原 command_id 查询回执，不自动重试。管理功能默认关闭。
// @Tags AI-Workflow-Publications
// @Accept json
// @Produce json
// @Param body body app.PublishConfiguration true "发布操作与版本确认"
// @Success 200 {object} core.Response{data=app.PublicationReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/publications/publish [post]
func (h *AIWorkflowPublicationHandler) Publish(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.PublishConfiguration
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Publish(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Rollback godoc
// @Summary 回退至历史 qs-ai 发布版本
// @Description 需要当前机构 OrgAdmin 权限；组织和操作人取认证上下文。必须确认预期选择器版本及当前发布 ID；超时后按原 command_id 查询回执，不自动重试。管理功能默认关闭。
// @Tags AI-Workflow-Publications
// @Accept json
// @Produce json
// @Param body body app.RollbackPublication true "发布操作与版本确认"
// @Success 200 {object} core.Response{data=app.PublicationReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/publications/rollback [post]
func (h *AIWorkflowPublicationHandler) Rollback(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.RollbackPublication
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Rollback(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Disable godoc
// @Summary 停用当前选择器的 qs-ai 发布配置
// @Description 需要当前机构 OrgAdmin 权限；组织和操作人取认证上下文。必须确认预期选择器版本及当前发布 ID；超时后按原 command_id 查询回执，不自动重试。管理功能默认关闭。
// @Tags AI-Workflow-Publications
// @Accept json
// @Produce json
// @Param body body app.PublicationCommand true "发布操作与版本确认"
// @Success 200 {object} core.Response{data=app.PublicationReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/publications/disable [post]
func (h *AIWorkflowPublicationHandler) Disable(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var command app.PublicationCommand
	if err := h.BindJSON(c, &command); err != nil {
		return
	}
	value, err := h.service.Disable(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// Get godoc
// @Summary 查询 qs-ai 配置选择器的当前发布
// @Description 需要当前机构解读审计权限。精确查询选择器，不执行运行时回退匹配。管理功能默认关闭。
// @Tags AI-Workflow-Publications
// @Produce json
// @Param audience query string true "participant"
// @Param model_kind query string true "scale"
// @Param decision_kind query string true "score_range"
// @Param model_code query string false "测评编码"
// @Param model_version query string false "测评版本，需要同时提供编码"
// @Success 200 {object} core.Response{data=app.PublicationState}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/publications [get]
func (h *AIWorkflowPublicationHandler) Get(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	selector := app.PublicationSelector{Audience: c.Query("audience"), ModelKind: c.Query("model_kind"), DecisionKind: c.Query("decision_kind")}
	// Preserve an explicitly empty optional field so validation rejects it.
	if value, exists := c.Request.URL.Query()["model_code"]; exists && len(value) > 0 {
		selector.ModelCode = &value[0]
	}
	if value, exists := c.Request.URL.Query()["model_version"]; exists && len(value) > 0 {
		selector.ModelVersion = &value[0]
	}
	value, err := h.service.Get(c.Request.Context(), scope, selector)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// GetReceipt godoc
// @Summary 查询 qs-ai 发布操作的原始回执
// @Description 需要当前机构解读审计权限；仅原组织和原操作人可读取。用于超时后确认原命令结果，查询不重放操作。
// @Tags AI-Workflow-Publications
// @Produce json
// @Param command_id path string true "原发布命令 UUID"
// @Success 200 {object} core.Response{data=app.PublicationReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/publications/commands/{command_id} [get]
func (h *AIWorkflowPublicationHandler) GetReceipt(c *gin.Context) {
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

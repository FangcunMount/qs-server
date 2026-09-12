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
)

type AIWorkflowSuiteHandler struct {
	*BaseHandler
	service *app.SuiteAdministration
}

func NewAIWorkflowSuiteHandler(s *app.SuiteAdministration) *AIWorkflowSuiteHandler {
	return &AIWorkflowSuiteHandler{NewBaseHandler(), s}
}
func (h *AIWorkflowSuiteHandler) scope(c *gin.Context) (app.DraftScope, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.DraftScope{}, false
	}
	return app.DraftScope{OrganizationID: org, OperatorUserID: user}, true
}
func (h *AIWorkflowSuiteHandler) failure(c *gin.Context, err error) {
	errorCode, message := code.ErrUnknown, "AI Suite outcome unknown; query the original command ID before saving again"
	switch {
	case errors.Is(err, app.ErrInvalid), status.Code(err) == codes.InvalidArgument:
		errorCode, message = code.ErrInvalidArgument, "Invalid AI Suite operation"
	case errors.Is(err, app.ErrGovernanceDenied), status.Code(err) == codes.PermissionDenied:
		errorCode, message = code.ErrPermissionDenied, "AI Suite governance permission required"
	case errors.Is(err, app.ErrNotFound), status.Code(err) == codes.NotFound:
		errorCode, message = code.ErrPageNotFound, "AI Suite revision or command receipt unavailable"
	case errors.Is(err, app.ErrConflict), status.Code(err) == codes.Aborted:
		errorCode, message = code.ErrConflict, "AI Suite version or confirmed state changed"
	case errors.Is(err, app.ErrManagementUnavailable):
		errorCode, message = code.ErrUnsupportedOperation, "AI Suite management disabled"
	}
	h.Error(c, cberrors.WithCode(errorCode, "%s", message))
}

// Register godoc
// @Summary 注册 qs-ai 不可变 Suite 版本
// @Description 需要当前机构 OrgAdmin 权限，组织和操作人来自认证上下文。注册保留案例并绑定配置，不代表评测批准或发布。超时后查询原 command_id，不自动重试。
// @Tags AI-Workflow-Suites
// @Accept json
// @Produce json
// @Param body body app.RegisterSuite true "原案例来源、新套件版本和确认的资产引用"
// @Success 200 {object} core.Response{data=app.SuiteRegistrationReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/suites/register [post]
func (h *AIWorkflowSuiteHandler) Register(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	var command app.RegisterSuite
	if err := c.ShouldBindJSON(&command); err != nil {
		h.failure(c, app.ErrInvalid)
		return
	}
	value, err := h.service.Register(c.Request.Context(), scope, command)
	if err != nil {
		h.failure(c, err)
		return
	}
	h.Success(c, value)
}

// GetReceipt godoc
// @Summary 查询 qs-ai Suite 注册原命令回执
// @Description 需要当前机构解读审计权限，仅原机构及原操作人可查询。不重发注册，不触发发布。
// @Tags AI-Workflow-Suites
// @Produce json
// @Param command_id path string true "原命令 UUID"
// @Success 200 {object} core.Response{data=app.SuiteRegistrationReceipt}
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-workflow/suites/commands/{command_id} [get]
func (h *AIWorkflowSuiteHandler) GetReceipt(c *gin.Context) {
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

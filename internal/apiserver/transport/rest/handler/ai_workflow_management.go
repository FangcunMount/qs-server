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

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

// aiWorkflowHandlerSupport shares request scope and error translation without
// borrowing another asset handler's service.
type aiWorkflowHandlerSupport struct {
	*BaseHandler
	asset string
}

func newAIWorkflowHandlerSupport(asset string) *aiWorkflowHandlerSupport {
	return &aiWorkflowHandlerSupport{BaseHandler: NewBaseHandler(), asset: asset}
}

func (h *aiWorkflowHandlerSupport) scope(c *gin.Context) (app.DraftScope, bool) {
	org, user, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return app.DraftScope{}, false
	}
	return app.DraftScope{OrganizationID: org, OperatorUserID: user}, true
}

func (h *aiWorkflowHandlerSupport) failure(c *gin.Context, err error) {
	errorCode, message := code.ErrUnknown, "AI "+h.asset+" outcome unknown; query the original command ID before saving again"
	switch {
	case errors.Is(err, app.ErrInvalid), status.Code(err) == codes.InvalidArgument:
		errorCode, message = code.ErrInvalidArgument, "Invalid AI "+h.asset+" operation"
	case errors.Is(err, app.ErrGovernanceDenied), status.Code(err) == codes.PermissionDenied:
		errorCode, message = code.ErrPermissionDenied, "AI "+h.asset+" governance permission required"
	case errors.Is(err, app.ErrNotFound), status.Code(err) == codes.NotFound:
		errorCode, message = code.ErrPageNotFound, "AI "+h.asset+" revision or command receipt unavailable"
	case errors.Is(err, app.ErrConflict), status.Code(err) == codes.Aborted:
		errorCode, message = code.ErrConflict, "AI "+h.asset+" version or confirmed state changed"
	case errors.Is(err, app.ErrManagementUnavailable):
		errorCode, message = code.ErrUnsupportedOperation, "AI "+h.asset+" management disabled"
	}
	h.Error(c, cberrors.WithCode(errorCode, "%s", message))
}

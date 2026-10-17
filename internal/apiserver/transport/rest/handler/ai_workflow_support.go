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
	errorCode, message := aiWorkflowFailureCode(err), "AI "+h.asset+" outcome unknown; query the original command ID before saving again"
	switch errorCode {
	case code.ErrInvalidArgument:
		message = "Invalid AI " + h.asset + " operation"
	case code.ErrPermissionDenied:
		message = "AI " + h.asset + " governance permission required"
	case code.ErrPageNotFound:
		message = "AI " + h.asset + " revision or command receipt unavailable"
	case code.ErrConflict:
		message = "AI " + h.asset + " version or confirmed state changed"
	case code.ErrUnsupportedOperation:
		message = "AI " + h.asset + " management disabled"
	}
	h.Error(c, cberrors.WithCode(errorCode, "%s", message))
}

func aiWorkflowFailureCode(err error) int {
	switch {
	case errors.Is(err, app.ErrInvalid), status.Code(err) == codes.InvalidArgument:
		return code.ErrInvalidArgument
	case errors.Is(err, app.ErrGovernanceDenied), status.Code(err) == codes.PermissionDenied:
		return code.ErrPermissionDenied
	case errors.Is(err, app.ErrNotFound), status.Code(err) == codes.NotFound:
		return code.ErrPageNotFound
	case errors.Is(err, app.ErrConflict), status.Code(err) == codes.Aborted:
		return code.ErrConflict
	case errors.Is(err, app.ErrManagementUnavailable):
		return code.ErrUnsupportedOperation
	default:
		return code.ErrUnknown
	}
}

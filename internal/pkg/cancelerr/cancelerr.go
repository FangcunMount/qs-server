package cancelerr

import (
	"context"
	stderrors "errors"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Is reports whether err represents request cancellation from local context
// propagation or a downstream gRPC call.
func Is(err error) bool {
	if err == nil {
		return false
	}
	if stderrors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
		return true
	}
	cause := cberrors.Cause(err)
	return cause != nil && (stderrors.Is(cause, context.Canceled) || status.Code(cause) == codes.Canceled)
}

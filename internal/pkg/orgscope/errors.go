package orgscope

import (
	"errors"
	"net/http"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HTTPStatusForResolveError maps resolver errors to HTTP status codes.
func HTTPStatusForResolveError(err error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	if errors.Is(err, ErrMismatch) || pkgerrors.IsCode(err, code.ErrPermissionDenied) {
		return http.StatusForbidden
	}
	if pkgerrors.IsCode(err, code.ErrInvalidArgument) {
		return http.StatusBadRequest
	}
	if errors.Is(err, ErrUnresolved) {
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}

// GRPCStatusForResolveError maps resolver errors to gRPC status errors.
func GRPCStatusForResolveError(err error) error {
	if err == nil {
		return status.Error(codes.InvalidArgument, "organization scope could not be resolved")
	}
	if errors.Is(err, ErrMismatch) || pkgerrors.IsCode(err, code.ErrPermissionDenied) {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	if pkgerrors.IsCode(err, code.ErrInvalidArgument) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if errors.Is(err, ErrUnresolved) {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	return status.Errorf(codes.Internal, "organization scope could not be resolved: %v", err)
}

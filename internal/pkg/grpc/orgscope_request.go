package grpc

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ResolveRequestOrgID returns QS org_id for gRPC handlers.
// When OrgScope interceptor resolved org into context, reqOrgID must match or be zero (uses context).
// When context has no org (service-to-service), reqOrgID must be non-zero.
func ResolveRequestOrgID(ctx context.Context, reqOrgID uint64) (uint64, error) {
	ctxOrgID, hasCtxOrg := OrgIDFromContext(ctx)
	if hasCtxOrg {
		if reqOrgID == 0 {
			return ctxOrgID, nil
		}
		if reqOrgID != ctxOrgID {
			return 0, status.Error(codes.PermissionDenied, "org_id does not match resolved organization scope")
		}
		return ctxOrgID, nil
	}
	if reqOrgID == 0 {
		return 0, status.Error(codes.InvalidArgument, "org_id is required")
	}
	return reqOrgID, nil
}

// ResolveRequestOrgIDInt64 is the int64 variant of ResolveRequestOrgID.
func ResolveRequestOrgIDInt64(ctx context.Context, reqOrgID int64) (int64, error) {
	if reqOrgID < 0 {
		return 0, status.Error(codes.InvalidArgument, "org_id is invalid")
	}
	var asUint uint64
	if reqOrgID > 0 {
		converted, err := safeconv.Int64ToUint64(reqOrgID)
		if err != nil {
			return 0, status.Errorf(codes.InvalidArgument, "org_id is invalid: %v", err)
		}
		asUint = converted
	}
	resolved, err := ResolveRequestOrgID(ctx, asUint)
	if err != nil {
		return 0, err
	}
	return safeconv.Uint64ToInt64(resolved)
}

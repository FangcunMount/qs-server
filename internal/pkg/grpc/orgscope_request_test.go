package grpc

import (
	"context"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestResolveRequestOrgIDUsesContextWhenRequestOmitsOrg(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), authContextKeyOrgID, uint64(88))

	got, err := ResolveRequestOrgID(ctx, 0)
	if err != nil {
		t.Fatalf("ResolveRequestOrgID() error = %v", err)
	}
	if got != 88 {
		t.Fatalf("org_id = %d, want 88", got)
	}
}

func TestResolveRequestOrgIDRejectsMismatchedRequestOrg(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), authContextKeyOrgID, uint64(88))

	_, err := ResolveRequestOrgID(ctx, 99)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("status = %v, want PermissionDenied", err)
	}
}

func TestResolveRequestOrgIDRequiresRequestOrgForServiceCalls(t *testing.T) {
	t.Parallel()
	_, err := ResolveRequestOrgID(context.Background(), 0)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status = %v, want InvalidArgument", err)
	}

	got, err := ResolveRequestOrgID(context.Background(), 9)
	if err != nil {
		t.Fatalf("ResolveRequestOrgID() error = %v", err)
	}
	if got != 9 {
		t.Fatalf("org_id = %d, want 9", got)
	}
}

func TestGRPCStatusForResolveErrorMapsPermissionDenied(t *testing.T) {
	t.Parallel()
	err := orgscope.GRPCStatusForResolveError(pkgerrors.WithCode(code.ErrPermissionDenied, "denied"))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("status = %v, want PermissionDenied", err)
	}
}

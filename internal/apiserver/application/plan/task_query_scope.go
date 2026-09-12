package plan

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
)

type taskScopeLister interface {
	ListStoreScopedTesteeIDs(context.Context, int64, int64, string, string) ([]uint64, error)
}

func (s *queryService) taskListScope(ctx context.Context, orgID int64) ([]uint64, error) {
	userID := actorctx.GrantingUserID(ctx)
	checker, ok := s.access.(taskScopeLister)
	if !ok || orgID <= 0 || actorctx.OperatorOrgID(ctx) != orgID || userID == 0 || userID > math.MaxInt64 {
		return nil, errors.WithCode(code.ErrPermissionDenied, "trusted operator and task list scope are required")
	}
	if err := appauthz.RequirePermission(ctx, appauthz.EvaluationPlanTaskResource, "list"); err != nil {
		return nil, err
	}
	return checker.ListStoreScopedTesteeIDs(ctx, orgID, int64(userID), appauthz.EvaluationPlanTaskResource, "list")
}

// The subject's participation is private even when the plan catalog is shared.
func (s *queryService) requireTesteeListScope(ctx context.Context, testeeID uint64, resource string) error {
	orgID := actorctx.OperatorOrgID(ctx)
	userID := actorctx.GrantingUserID(ctx)
	if s.access == nil || orgID <= 0 || userID == 0 || userID > math.MaxInt64 || testeeID == 0 {
		return errors.WithCode(code.ErrPermissionDenied, "trusted operator and testee scope are required")
	}
	if err := appauthz.RequirePermission(ctx, resource, "list"); err != nil {
		return err
	}
	return s.access.ValidateTesteeStoreAccess(ctx, orgID, int64(userID), testeeID, resource, "list")
}

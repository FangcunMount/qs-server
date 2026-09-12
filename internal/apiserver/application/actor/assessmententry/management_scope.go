package assessmententry

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
)

type ManagementScope interface {
	ResolveStoreRange(context.Context, int64, int64, string, string) (authz.StoreRange, error)
}

// Entry administration is part of headquarters clinician configuration.
// Public Resolve/Intake never invoke this guard; they retain their own checks.
func (s *service) authorizeManagement(ctx context.Context, orgID int64, action string) error {
	if !s.managed {
		return nil
	}
	user := actorctx.GrantingUserID(ctx)
	snapshot, ok := authz.FromContext(ctx)
	if s.managementScope == nil || orgID <= 0 || actorctx.OperatorOrgID(ctx) != orgID || user == 0 || user > math.MaxInt64 || !ok || snapshot == nil || !snapshot.IsQSAdmin() {
		return errors.WithCode(code.ErrPermissionDenied, "company administrator scope required")
	}
	scope, err := s.managementScope.ResolveStoreRange(ctx, orgID, int64(user), "qs:actor:collection:clinicians", action)
	if err != nil {
		return err
	}
	if !scope.AllStores {
		return errors.WithCode(code.ErrPermissionDenied, "headquarters company scope required")
	}
	return nil
}

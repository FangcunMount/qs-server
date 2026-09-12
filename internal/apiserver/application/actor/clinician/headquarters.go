package clinician

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
)

func authorizeHeadquarters(ctx context.Context, scope SummaryScope, orgID int64, action string) error {
	user := actorctx.GrantingUserID(ctx)
	snapshot, ok := appauthz.FromContext(ctx)
	if scope == nil || orgID <= 0 || actorctx.OperatorOrgID(ctx) != orgID || user == 0 || user > math.MaxInt64 || !ok || snapshot == nil || !snapshot.IsQSAdmin() {
		return errors.WithCode(code.ErrPermissionDenied, "company administrator scope required")
	}
	allowed, err := scope.ResolveStoreRange(ctx, orgID, int64(user), "qs:actor:collection:clinicians", action)
	if err != nil {
		return err
	}
	if !allowed.AllStores {
		return errors.WithCode(code.ErrPermissionDenied, "headquarters company scope required")
	}
	return nil
}

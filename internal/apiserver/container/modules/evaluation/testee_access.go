package evaluation

import (
	"context"

	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
)

type testeeAccessChecker struct {
	delegate actorAccessApp.TesteeAccessService
}

// NewTesteeAccessChecker adapts actor testee access to evaluation assessment checks.
func NewTesteeAccessChecker(delegate actorAccessApp.TesteeAccessService) evaluationoperator.AccessChecker {
	if delegate == nil {
		return nil
	}
	return testeeAccessChecker{delegate: delegate}
}

func (c testeeAccessChecker) ResolveAccessScope(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
) (*evaluationoperator.AccessScope, error) {
	scope, err := c.delegate.ResolveAccessScope(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		return nil, nil
	}
	return &evaluationoperator.AccessScope{
		IsAdmin:     scope.IsAdmin,
		ClinicianID: scope.ClinicianID,
	}, nil
}

func (c testeeAccessChecker) ValidateTesteeAccess(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	testeeID uint64,
) error {
	return c.delegate.ValidateTesteeAccess(ctx, orgID, operatorUserID, testeeID)
}

func (c testeeAccessChecker) ListAccessibleTesteeIDs(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
) ([]uint64, error) {
	return c.delegate.ListAccessibleTesteeIDs(ctx, orgID, operatorUserID)
}

package assembler

import (
	"context"

	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
)

type evaluationTesteeAccessChecker struct {
	delegate actorAccessApp.TesteeAccessService
}

func newEvaluationTesteeAccessChecker(delegate actorAccessApp.TesteeAccessService) assessmentApp.TesteeAccessChecker {
	if delegate == nil {
		return nil
	}
	return evaluationTesteeAccessChecker{delegate: delegate}
}

func (c evaluationTesteeAccessChecker) ResolveAccessScope(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
) (*assessmentApp.TesteeAccessScope, error) {
	scope, err := c.delegate.ResolveAccessScope(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		return nil, nil
	}
	return &assessmentApp.TesteeAccessScope{
		IsAdmin:     scope.IsAdmin,
		ClinicianID: scope.ClinicianID,
	}, nil
}

func (c evaluationTesteeAccessChecker) ValidateTesteeAccess(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
	testeeID uint64,
) error {
	return c.delegate.ValidateTesteeAccess(ctx, orgID, operatorUserID, testeeID)
}

func (c evaluationTesteeAccessChecker) ListAccessibleTesteeIDs(
	ctx context.Context,
	orgID int64,
	operatorUserID int64,
) ([]uint64, error) {
	return c.delegate.ListAccessibleTesteeIDs(ctx, orgID, operatorUserID)
}

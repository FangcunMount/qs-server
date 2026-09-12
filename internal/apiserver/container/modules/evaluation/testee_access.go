package evaluation

import (
	"context"
	"fmt"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"

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

func (c testeeAccessChecker) ValidateTesteeStoreAccess(ctx context.Context, orgID, userID int64, testeeID uint64, resource, action string) error {
	checker, ok := c.delegate.(actorAccessApp.StoreScopeAccess)
	if !ok {
		return fmt.Errorf("store scope access checker is not configured")
	}
	return checker.ValidateTesteeStoreAccess(ctx, orgID, userID, testeeID, resource, action)
}

func (c testeeAccessChecker) ResolveStoreRange(ctx context.Context, orgID, userID int64, resource, action string) (appauthz.StoreRange, error) {
	checker, ok := c.delegate.(actorAccessApp.StoreScopeAccess)
	if !ok {
		return appauthz.StoreRange{}, fmt.Errorf("store scope access checker is not configured")
	}
	return checker.ResolveStoreRange(ctx, orgID, userID, resource, action)
}

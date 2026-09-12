package access

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	rm "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// StoreScopeAccess checks an explicit action's range for backend operators.
// Clinician and guardian self-service use their independent relationship rules.
type StoreScopeAccess interface {
	ListStoreScopedTesteeIDs(context.Context, int64, int64, string, string) ([]uint64, error)
	ResolveStoreRange(context.Context, int64, int64, string, string) (authz.StoreRange, error)
	ValidateTesteeStoreAccess(context.Context, int64, int64, uint64, string, string) error
}

var _ StoreScopeAccess = (*service)(nil)

func (s *service) ResolveStoreRange(ctx context.Context, orgID, userID int64, resource, action string) (authz.StoreRange, error) {
	denied := func() (authz.StoreRange, error) {
		return authz.StoreRange{}, errors.WithCode(code.ErrPermissionDenied, "active operator and scoped authorization required")
	}
	if orgID <= 0 || userID <= 0 || s.operatorReader == nil {
		return denied()
	}
	operator, err := s.operatorReader.FindOperatorByUser(ctx, orgID, userID)
	if err != nil {
		return authz.StoreRange{}, err
	}
	if operator == nil || !operator.IsActive || operator.OrgID != orgID || operator.UserID != userID {
		return denied()
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok {
		return denied()
	}
	return snapshot.ResolveStoreRange(orgID, resource, action)
}

func (s *service) ValidateTesteeStoreAccess(ctx context.Context, orgID, userID int64, testeeID uint64, resource, action string) error {
	stores, err := s.ResolveStoreRange(ctx, orgID, userID, resource, action)
	if err != nil {
		return err
	}
	if testeeID == 0 || s.testeeReader == nil {
		return errors.WithCode(code.ErrPermissionDenied, "testee ownership required")
	}
	if err := ensureAccessIDFromUint64("testee_id", testeeID); err != nil {
		return err
	}
	row, err := s.testeeReader.GetTestee(ctx, testeeID)
	if err != nil {
		return err
	}
	if row == nil || row.OrgID != orgID || !stores.Contains(row.StoreID) {
		return errors.WithCode(code.ErrPermissionDenied, "testee outside granted store range")
	}
	return nil
}

func (s *service) ListStoreScopedTesteeIDs(ctx context.Context, orgID, userID int64, resource, action string) ([]uint64, error) {
	stores, err := s.ResolveStoreRange(ctx, orgID, userID, resource, action)
	if err != nil {
		return nil, err
	}
	selector, ok := s.testeeReader.(rm.TesteeStoreSelector)
	if !ok {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "current testee ownership selector is not configured")
	}
	return selector.ListTesteeIDsInStores(ctx, orgID, stores.StoreIDs, stores.AllStores)
}

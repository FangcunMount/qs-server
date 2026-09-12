package access

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	rm "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStoreAccessRequiresOperatorAndActionSpecificCurrentOwnership(t *testing.T) {
	store := uint64(7)
	operators := &stubOperatorReader{item: rm.OperatorRow{OrgID: 1, UserID: 101, IsActive: true}}
	testees := &stubTesteeReader{item: rm.TesteeRow{ID: 401, OrgID: 1, StoreID: &store}}
	svc := NewTesteeAccessService(operators, testees).(StoreScopeAccess)
	snapshot := &authz.Snapshot{AuthzVersion: 1, ScopeContractVersion: 1, Permissions: []authz.Permission{
		{Resource: authz.AssessmentResource, Action: "read", Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}}},
		{Resource: authz.AssessmentResource, Action: "retry", Mode: authz.AuthorizationModeUnconditional, Scopes: []authz.DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{8}}}},
	}}
	ctx := authz.WithSnapshot(context.Background(), snapshot)
	check := func(action string) error {
		return svc.ValidateTesteeStoreAccess(ctx, 1, 101, 401, authz.AssessmentResource, action)
	}
	require.NoError(t, check("read"))
	require.Error(t, check("retry"))
	store = 8
	require.Error(t, check("read"))
	require.NoError(t, check("retry"))
	operators.item.IsActive = false
	require.Error(t, check("retry"))
	operators.item.IsActive = true
	testees.item.StoreID = nil
	require.Error(t, check("retry"))
	testees.item.StoreID = &store
	testees.item.OrgID = 2
	require.Error(t, check("retry"))
	require.Error(t, svc.ValidateTesteeStoreAccess(context.Background(), 1, 101, 401, authz.AssessmentResource, "retry"))
}

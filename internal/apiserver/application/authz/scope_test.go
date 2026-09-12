package authz

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRangeUnionDoesNotCrossActionOrCompany(t *testing.T) {
	s := &Snapshot{ScopeContractVersion: 1, AuthzVersion: 7, Permissions: []Permission{
		{Resource: AssessmentResource, Action: "retry", Mode: AuthorizationModeUnconditional, Scopes: []DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{10}}}},
		{Resource: AssessmentResource, Action: "read", Mode: AuthorizationModeUnconditional, Scopes: []DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{20}}, {OrgID: 2, Kind: "all_stores"}}},
	}}
	r, err := s.ResolveStoreRange(1, AssessmentResource, "retry")
	require.NoError(t, err)
	require.Equal(t, []uint64{10}, r.StoreIDs)
	other := uint64(20)
	require.False(t, r.Contains(&other))
	require.False(t, r.Contains(nil))
	_, err = s.ResolveStoreRange(2, AssessmentResource, "retry")
	require.Error(t, err)
	s.Permissions = append(s.Permissions, Permission{Resource: AssessmentResource, Action: "retry", Mode: AuthorizationModeUnconditional, Scopes: []DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{20, 10}}}})
	r, err = s.ResolveStoreRange(1, AssessmentResource, "retry")
	require.NoError(t, err)
	require.Equal(t, []uint64{10, 20}, r.StoreIDs)
}

func TestRangeRequiresScopeAwareSnapshotAndExplicitRange(t *testing.T) {
	s := &Snapshot{AuthzVersion: 1, Permissions: []Permission{{Resource: "qs:*:*:*", Action: "*", Mode: AuthorizationModeUnconditional}}}
	_, err := s.ResolveStoreRange(1, AssessmentResource, "read")
	require.Error(t, err)
	s.ScopeContractVersion = 1
	_, err = s.ResolveStoreRange(1, AssessmentResource, "read")
	require.Error(t, err)
	s.Permissions[0].Scopes = []DataScope{{OrgID: 1, Kind: "all_stores"}}
	r, err := s.ResolveStoreRange(1, AssessmentResource, "read")
	require.NoError(t, err)
	require.True(t, r.AllStores)
	require.False(t, r.Contains(nil))
	s.Permissions[0].Scopes[0].Kind = "unknown"
	_, err = s.ResolveStoreRange(1, AssessmentResource, "read")
	require.Error(t, err)
}

package iam

import (
	pb "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOperatorAssignmentFactsKeepForeignCompanyAndUnconfiguredFacts(t *testing.T) {
	role := &pb.AssignmentRoleFact{RoleId: "1", RoleName: "qs:assessment_operator", ManagementProtection: "standard"}
	response := &pb.GetAuthorizationSnapshotResponse{PolicyVersion: 9, ScopeContractVersion: 1, AssignmentFactsComplete: true, AssignmentFacts: []*pb.AssignmentRoleFact{role}, AssignmentScopes: []*pb.AssignmentScopeFact{
		{AssignmentId: "10", Role: role, Scope: &pb.DataScope{OrgId: "1", Kind: pb.DataScopeKind_STORES, StoreIds: []string{"123456789012345678"}}},
		{AssignmentId: "11", Role: role, Scope: &pb.DataScope{OrgId: "2", Kind: pb.DataScopeKind_ALL_STORES}},
		{AssignmentId: "12", Role: role},
	}}
	result, err := operatorAssignmentFacts(response)
	require.NoError(t, err)
	require.EqualValues(t, 9, result.PolicyVersion)
	require.Len(t, result.Assignments, 3)
	require.Equal(t, []uint64{123456789012345678}, result.Assignments[0].Scope.StoreIDs)
	require.EqualValues(t, 2, result.Assignments[1].Scope.OrgID)
	require.Nil(t, result.Assignments[2].Scope)
	response.AssignmentScopes[0].Scope.StoreIds[0] = "2"
	require.EqualValues(t, 123456789012345678, result.Assignments[0].Scope.StoreIDs[0])
	response.AssignmentScopes = nil
	_, err = operatorAssignmentFacts(response)
	require.Error(t, err)
}

func TestScopedRoleRequestPreservesCompanyAndRejectsImplicitRange(t *testing.T) {
	selected := iambridge.OperatorScopedRole{RoleName: "qs:assessment_operator", Scope: iambridge.OperatorAssignmentScope{OrgID: 1, Kind: "stores", StoreIDs: []uint64{123456789012345678}}}
	entries, err := operatorScopedRoleRequests(1, []iambridge.OperatorScopedRole{selected})
	require.NoError(t, err)
	require.Equal(t, "1", entries[0].Scope.OrgId)
	require.Equal(t, []string{"123456789012345678"}, entries[0].Scope.StoreIds)
	_, err = operatorScopedRoleRequests(2, []iambridge.OperatorScopedRole{selected})
	require.Error(t, err)
	_, err = operatorScopedRoleRequests(1, []iambridge.OperatorScopedRole{selected, selected})
	require.Error(t, err)
	for _, value := range []iambridge.OperatorAssignmentScope{{OrgID: 1}, {OrgID: 1, Kind: "stores"}, {OrgID: 1, Kind: "stores", StoreIDs: []uint64{0}}, {OrgID: 1, Kind: "all_stores", StoreIDs: []uint64{7}}} {
		bad := selected
		bad.Scope = value
		_, err := operatorScopedRoleRequests(1, []iambridge.OperatorScopedRole{bad})
		require.Error(t, err)
	}
	entries, err = operatorScopedRoleRequests(1, nil)
	require.NoError(t, err)
	require.Empty(t, entries, "empty explicit set revokes current company managed assignments")
}

func TestOperatorProjectionUsesOnlyCurrentCompanyConfiguredRoles(t *testing.T) {
	facts := iambridge.OperatorAssignmentFacts{PolicyVersion: 9, Assignments: []iambridge.OperatorAssignmentFact{
		{RoleName: "qs:assessment_operator", ManagementProtection: "standard", Scope: &iambridge.OperatorAssignmentScope{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}},
		{RoleName: "qs:result_reviewer", ManagementProtection: "standard", Scope: &iambridge.OperatorAssignmentScope{OrgID: 2, Kind: "all_stores"}},
		{RoleName: "qs:content_manager", ManagementProtection: "standard"},
		{RoleName: "platform_admin", ManagementProtection: "protected"},
	}}
	result, err := scopedOperatorProjection(1, facts)
	require.NoError(t, err)
	require.Equal(t, []string{"qs:assessment_operator"}, result.DirectRoles)
	require.Equal(t, result.DirectRoles, result.EffectiveRoles)
	require.True(t, result.ProtectedAccess)
	require.EqualValues(t, 9, result.PolicyVersion)
}

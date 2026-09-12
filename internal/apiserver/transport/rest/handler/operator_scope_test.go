package handler

import (
	"encoding/json"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/request"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOperatorScopeResponseKeepsStringIDsAndExplicitNull(t *testing.T) {
	value := scopeFactResponse(iambridge.OperatorAssignmentFact{AssignmentID: "123456789012345679", Scope: &iambridge.OperatorAssignmentScope{OrgID: 1, Kind: "stores", StoreIDs: []uint64{123456789012345678}}})
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.Contains(t, string(data), `"store_ids":["123456789012345678"]`)
	require.Contains(t, string(data), `"org_id":"1"`)
	require.Contains(t, string(data), `"assignment_id":"123456789012345679"`)
	data, err = json.Marshal(scopeFactResponse(iambridge.OperatorAssignmentFact{}))
	require.NoError(t, err)
	require.Contains(t, string(data), `"scope":null`)
}

func TestScopeUpdateInputRequiresExplicitRolesAndLosslessVersion(t *testing.T) {
	roles := []request.OperatorScopeRole{{RoleName: "qs:assessment_operator", Kind: "stores", StoreIDs: []string{"123456789012345678"}}}
	req := request.ReplaceOperatorScopeRequest{ExpectedPolicyVersion: "123456789012345679", Roles: &roles, Reason: "test"}
	input, err := scopeUpdateInput(10, 1, req)
	require.NoError(t, err)
	require.EqualValues(t, 123456789012345679, input.ExpectedPolicyVersion)
	require.EqualValues(t, 1, input.Roles[0].Scope.OrgID)
	require.EqualValues(t, 123456789012345678, input.Roles[0].Scope.StoreIDs[0])
	req.Roles = nil
	_, err = scopeUpdateInput(10, 1, req)
	require.Error(t, err)
	empty := []request.OperatorScopeRole{}
	req.Roles = &empty
	input, err = scopeUpdateInput(10, 1, req)
	require.NoError(t, err)
	require.Empty(t, input.Roles)
	for _, version := range []string{"", "0", "-1", "01", "9223372036854775808"} {
		req.ExpectedPolicyVersion = version
		_, err = scopeUpdateInput(10, 1, req)
		require.Error(t, err)
	}
}

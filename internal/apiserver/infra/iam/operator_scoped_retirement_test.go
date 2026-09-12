package iam

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"testing"
)

type retirementScopeStub struct {
	iambridge.OperatorScopeGateway
	facts   iambridge.OperatorAssignmentFacts
	writes  int
	version int64
}

func (s *retirementScopeStub) LoadOperatorAssignmentFacts(context.Context, int64) (iambridge.OperatorAssignmentFacts, error) {
	return s.facts, nil
}
func (s *retirementScopeStub) ReplaceOperatorScopedRoles(_ context.Context, org, user int64, roles []iambridge.OperatorScopedRole, version int64, by, reason string) (int64, error) {
	if org != 7 || user != 9 || len(roles) != 0 || by != "user:1" || reason != "exit" {
		panic("invalid scoped revocation")
	}
	s.writes++
	s.version = version
	return version + 1, nil
}
func TestScopedRetirementRechecksFactsAndUsesVersion(t *testing.T) {
	for _, kind := range []string{"valid", "foreign", "legacy", "protected", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			fact := iambridge.OperatorAssignmentFact{RoleName: "qs:result_reviewer", ManagementProtection: "standard", Scope: &iambridge.OperatorAssignmentScope{OrgID: 7, Kind: "all_stores"}}
			stub := &retirementScopeStub{facts: iambridge.OperatorAssignmentFacts{PolicyVersion: 12, Assignments: []iambridge.OperatorAssignmentFact{fact}}}
			gateway := &scopedRetirementGateway{gateway: stub}
			if _, err := gateway.LoadOperatorRoleProjection(context.Background(), 7, 9); err != nil {
				t.Fatal(err)
			}
			// Facts may change between initial inspection and revocation.
			switch kind {
			case "foreign":
				stub.facts.Assignments[0].Scope.OrgID = 8
			case "legacy":
				stub.facts.Assignments[0].Scope = nil
			case "protected":
				stub.facts.Assignments[0].ManagementProtection = "protected"
			case "unknown":
				stub.facts.Assignments[0].RoleName = "unexpected"
			}
			version, err := gateway.ReplaceManagedOperatorRoles(context.Background(), 7, 9, nil, "user:1", "exit")
			if kind == "valid" {
				if err != nil || version != 13 || stub.writes != 1 || stub.version != 12 {
					t.Fatalf("revoke %d %v %+v", version, err, stub)
				}
			} else if err == nil || stub.writes != 0 {
				t.Fatal("conflicting facts revoked")
			}
		})
	}
}

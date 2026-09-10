package operator

import "testing"

func TestOperatorRoleProjectionIsCopied(t *testing.T) {
	item := NewOperator(1, 10, "operator")
	item.ReplaceRolesProjection([]Role{RoleAssessmentOperator}, []Role{RoleAssessmentOperator}, 1, nil, false)

	roles := item.Roles()
	if len(roles) != 1 || roles[0] != RoleAssessmentOperator {
		t.Fatalf("expected one staff role, got %v", roles)
	}
}

func TestOperatorRolesReturnsCopy(t *testing.T) {
	item := NewOperator(1, 10, "operator")
	item.ReplaceRolesProjection([]Role{RoleAssessmentOperator}, []Role{RoleAssessmentOperator}, 1, nil, false)

	roles := item.Roles()
	roles[0] = RoleQSAdmin

	current := item.Roles()
	if len(current) != 1 || current[0] != RoleAssessmentOperator {
		t.Fatalf("expected returned roles slice not to mutate aggregate")
	}
}

func TestReplaceRolesProjectionIgnoresInheritedEffectiveArgument(t *testing.T) {
	item := NewOperator(1, 10, "operator")
	item.ReplaceRolesProjection(
		[]Role{RoleAssessmentOperator},
		[]Role{RoleAssessmentOperator, RoleResultReviewer},
		7,
		nil,
		false,
	)
	if got := item.EffectiveRoles(); len(got) != 1 || got[0] != RoleAssessmentOperator {
		t.Fatalf("effective roles = %v, want direct roles only", got)
	}
}

func TestIsSupportedRoleRejectsRetiredRoles(t *testing.T) {
	for _, role := range []Role{"qs:staff", "qs:evaluator", "super_admin"} {
		if IsSupportedRole(role) {
			t.Fatalf("%s must not remain assignable", role)
		}
	}
	for _, role := range []Role{RoleAssessmentOperator, RoleResultReviewer, RoleQSAdmin, RoleContentManager, RoleEvaluationPlanManager} {
		if !IsSupportedRole(role) {
			t.Fatalf("%s must remain supported", role)
		}
	}
}

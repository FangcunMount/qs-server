package operator

import "testing"

func TestOperatorAssignRoleIsIdempotent(t *testing.T) {
	item := NewOperator(1, 10, "operator")

	if err := item.AssignRole(RoleOperator); err != nil {
		t.Fatalf("AssignRole returned error: %v", err)
	}
	if err := item.AssignRole(RoleOperator); err != nil {
		t.Fatalf("AssignRole returned error on duplicate: %v", err)
	}

	roles := item.Roles()
	if len(roles) != 1 || roles[0] != RoleOperator {
		t.Fatalf("expected one staff role, got %v", roles)
	}
}

func TestOperatorReplaceRolesDeduplicatesInInputOrder(t *testing.T) {
	item := NewOperator(1, 10, "operator")

	if err := item.ReplaceRoles([]Role{RoleEvaluatorQS, RoleOperator, RoleEvaluatorQS}); err != nil {
		t.Fatalf("ReplaceRoles returned error: %v", err)
	}

	roles := item.Roles()
	expected := []Role{RoleEvaluatorQS, RoleOperator}
	if len(roles) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, roles)
	}
	for i := range expected {
		if roles[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, roles)
		}
	}
}

func TestOperatorRejectsRoleChangesWhenInactive(t *testing.T) {
	item := NewOperator(1, 10, "operator")
	item.deactivate()

	if err := item.AssignRole(RoleOperator); err == nil {
		t.Fatalf("expected AssignRole to reject inactive operator")
	}
	if err := item.ReplaceRoles([]Role{RoleOperator}); err == nil {
		t.Fatalf("expected ReplaceRoles to reject inactive operator")
	}
}

func TestOperatorRolesReturnsCopy(t *testing.T) {
	item := NewOperator(1, 10, "operator")
	if err := item.AssignRole(RoleOperator); err != nil {
		t.Fatalf("AssignRole returned error: %v", err)
	}

	roles := item.Roles()
	roles[0] = RoleQSAdmin

	if !item.HasRole(RoleOperator) || item.HasRole(RoleQSAdmin) {
		t.Fatalf("expected returned roles slice not to mutate aggregate")
	}
}

package operator

import "testing"

func TestOperatorRoleProjectionIsCopied(t *testing.T) {
	item := NewOperator(1, 10, "operator")
	item.ReplaceRolesProjection([]Role{RoleOperator}, []Role{RoleOperator}, 1, nil, false)

	roles := item.Roles()
	if len(roles) != 1 || roles[0] != RoleOperator {
		t.Fatalf("expected one staff role, got %v", roles)
	}
}

func TestOperatorRolesReturnsCopy(t *testing.T) {
	item := NewOperator(1, 10, "operator")
	item.ReplaceRolesProjection([]Role{RoleOperator}, []Role{RoleOperator}, 1, nil, false)

	roles := item.Roles()
	roles[0] = RoleQSAdmin

	current := item.Roles()
	if len(current) != 1 || current[0] != RoleOperator {
		t.Fatalf("expected returned roles slice not to mutate aggregate")
	}
}

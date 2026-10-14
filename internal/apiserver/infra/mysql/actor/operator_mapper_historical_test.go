package actor

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
)

func TestOperatorMapperAndReadModelDeriveEffectiveRolesFromDirectRoles(t *testing.T) {
	mapper := NewOperatorMapper()
	historical := &OperatorPO{
		OrgID:              1,
		UserID:             10,
		Name:               "operator",
		Roles:              StringSliceCol{string(domain.RoleAssessmentOperator)},
		AuthzPolicyVersion: 7,
		IsActive:           true,
	}

	restored := mapper.ToDomain(historical)
	if got := restored.EffectiveRoles(); len(got) != 1 || got[0] != domain.RoleAssessmentOperator {
		t.Fatalf("historical inherited roles entered the projection: %v", got)
	}
	persisted := mapper.ToPO(restored)
	if got := persisted.Roles; len(got) != 1 || got[0] != string(domain.RoleAssessmentOperator) {
		t.Fatalf("canonical column must contain direct roles only: %v", got)
	}
	row := operatorRowFromPO(historical)
	if len(row.EffectiveRoles) != 1 || row.EffectiveRoles[0] != string(domain.RoleAssessmentOperator) {
		t.Fatal("read model must derive effective roles from direct roles")
	}
	row.EffectiveRoles[0] = string(domain.RoleQSAdmin)
	if historical.Roles[0] != string(domain.RoleAssessmentOperator) {
		t.Fatal("derived role slice aliases persisted roles")
	}
	if restored.AuthzPolicyVersion() != historical.AuthzPolicyVersion || !restored.IsActive() {
		t.Fatal("restoring historical roles changed projection version or activity")
	}
}

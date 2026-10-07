package actor

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
)

func TestOperatorMapperKeepsHistoricalInheritedRolesOutOfProjection(t *testing.T) {
	mapper := NewOperatorMapper()
	historical := &OperatorPO{
		OrgID:              1,
		UserID:             10,
		Name:               "operator",
		Roles:              StringSliceCol{string(domain.RoleAssessmentOperator)},
		EffectiveRoles:     StringSliceCol{string(domain.RoleAssessmentOperator), string(domain.RoleQSAdmin)},
		AuthzPolicyVersion: 7,
		IsActive:           true,
	}

	restored := mapper.ToDomain(historical)
	if got := restored.EffectiveRoles(); len(got) != 1 || got[0] != domain.RoleAssessmentOperator {
		t.Fatalf("historical inherited roles entered the projection: %v", got)
	}
	persisted := mapper.ToPO(restored)
	if got := persisted.EffectiveRoles; len(got) != 1 || got[0] != string(domain.RoleAssessmentOperator) {
		t.Fatalf("compatibility column must contain direct roles only: %v", got)
	}
	if restored.AuthzPolicyVersion() != historical.AuthzPolicyVersion || !restored.IsActive() {
		t.Fatal("restoring historical roles changed projection version or activity")
	}
}

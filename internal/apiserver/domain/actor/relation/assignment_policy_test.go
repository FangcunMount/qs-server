package relation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func TestAssignmentPolicyReusesExistingPrimaryForSameClinician(t *testing.T) {
	existing := makeAssignmentPolicyRelation(10, 20, RelationTypePrimary)
	policy := NewAssignmentPolicy()

	plan, err := policy.PlanAssignment(makeAssignmentRequest(10, 20, RelationTypePrimary), existing, nil)
	if err != nil {
		t.Fatalf("PlanAssignment returned error: %v", err)
	}
	if plan.ReuseRelation != existing {
		t.Fatalf("expected existing primary relation to be reused")
	}
	if len(plan.Unbind) != 0 || plan.Create != nil {
		t.Fatalf("expected no unbind or create, got %+v", plan)
	}
}

func TestAssignmentPolicyReplacesPrimaryAndExistingAccessRelation(t *testing.T) {
	activePrimary := makeAssignmentPolicyRelation(10, 20, RelationTypePrimary)
	activeAccess := makeAssignmentPolicyRelation(11, 20, RelationTypeAttending)
	policy := NewAssignmentPolicy()

	plan, err := policy.PlanAssignment(makeAssignmentRequest(11, 20, RelationTypePrimary), activePrimary, activeAccess)
	if err != nil {
		t.Fatalf("PlanAssignment returned error: %v", err)
	}
	if plan.ReuseRelation != nil {
		t.Fatalf("expected no reused relation")
	}
	if len(plan.Unbind) != 2 || plan.Unbind[0] != activePrimary || plan.Unbind[1] != activeAccess {
		t.Fatalf("expected primary and access relations to be unbound, got %+v", plan.Unbind)
	}
	if activePrimary.IsActive() || activeAccess.IsActive() {
		t.Fatalf("expected replaced relations to be marked inactive")
	}
	if plan.Create == nil || plan.Create.RelationType() != RelationTypePrimary || plan.Create.ClinicianID() != clinician.ID(11) {
		t.Fatalf("expected new primary relation, got %+v", plan.Create)
	}
}

func TestAssignmentPolicyReusesExistingAccessRelationWithSameType(t *testing.T) {
	activeAccess := makeAssignmentPolicyRelation(11, 20, RelationTypeAttending)
	policy := NewAssignmentPolicy()

	plan, err := policy.PlanAssignment(makeAssignmentRequest(11, 20, RelationTypeAttending), nil, activeAccess)
	if err != nil {
		t.Fatalf("PlanAssignment returned error: %v", err)
	}
	if plan.ReuseRelation != activeAccess {
		t.Fatalf("expected existing access relation to be reused")
	}
	if len(plan.Unbind) != 0 || plan.Create != nil {
		t.Fatalf("expected no unbind or create, got %+v", plan)
	}
}

func makeAssignmentRequest(clinicianID, testeeID uint64, relationType RelationType) AssignmentRequest {
	return AssignmentRequest{
		OrgID:        1,
		ClinicianID:  clinician.ID(clinicianID),
		TesteeID:     testee.ID(testeeID),
		RelationType: relationType,
		SourceType:   SourceTypeManual,
		Now:          time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
	}
}

func makeAssignmentPolicyRelation(clinicianID, testeeID uint64, relationType RelationType) *ClinicianTesteeRelation {
	return NewClinicianTesteeRelation(
		1,
		clinician.ID(clinicianID),
		testee.ID(testeeID),
		relationType,
		SourceTypeManual,
		nil,
		true,
		time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		nil,
	)
}

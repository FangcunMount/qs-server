package relation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func TestClinicianTesteeRelationUnbind(t *testing.T) {
	boundAt := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	item := NewClinicianTesteeRelation(
		1,
		clinician.ID(10),
		testee.ID(11),
		RelationTypeAssigned,
		SourceTypeManual,
		nil,
		true,
		boundAt,
		nil,
	)

	unboundAt := boundAt.Add(2 * time.Hour)
	item.Unbind(unboundAt)

	if item.IsActive() {
		t.Fatalf("expected relation to be inactive after unbind")
	}
	if item.UnboundAt() == nil || !item.UnboundAt().Equal(unboundAt) {
		t.Fatalf("expected unbound_at to be recorded")
	}
}

func TestGrantsAccess(t *testing.T) {
	cases := []struct {
		relationType RelationType
		expected     bool
	}{
		{relationType: RelationTypeAssigned, expected: true},
		{relationType: RelationTypePrimary, expected: true},
		{relationType: RelationTypeAttending, expected: true},
		{relationType: RelationTypeCollaborator, expected: true},
		{relationType: RelationTypeCreator, expected: false},
	}

	for _, tc := range cases {
		if actual := GrantsAccess(tc.relationType); actual != tc.expected {
			t.Fatalf("expected GrantsAccess(%s)=%v, got %v", tc.relationType, tc.expected, actual)
		}
	}
}

func TestNormalizeAssignableRelationType(t *testing.T) {
	if got := NormalizeAssignableRelationType(""); got != RelationTypeAttending {
		t.Fatalf("expected empty relation type to normalize to attending, got %s", got)
	}
	if got := NormalizeAssignableRelationType(RelationTypeAssigned); got != RelationTypeAttending {
		t.Fatalf("expected assigned to normalize to attending, got %s", got)
	}
	if got := NormalizeAssignableRelationType(RelationTypePrimary); got != RelationTypePrimary {
		t.Fatalf("expected primary to remain primary, got %s", got)
	}
}

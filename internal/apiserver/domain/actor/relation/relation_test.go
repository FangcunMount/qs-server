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

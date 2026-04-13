package assessmententry

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
)

func TestAssessmentEntryLifecycle(t *testing.T) {
	now := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	item := NewAssessmentEntry(1, clinician.ID(9), "token-1", TargetTypeQuestionnaire, "q1", "v1", true, &expiresAt)

	if !item.CanResolve(now) {
		t.Fatalf("expected active non-expired entry to resolve")
	}

	item.Deactivate()
	if item.CanResolve(now) {
		t.Fatalf("expected deactivated entry to stop resolving")
	}

	item.Reactivate()
	if !item.CanResolve(now) {
		t.Fatalf("expected reactivated entry to resolve")
	}

	if item.CanResolve(expiresAt.Add(time.Second)) {
		t.Fatalf("expected expired entry to stop resolving")
	}
}

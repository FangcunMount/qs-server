package evaluationinput

import (
	"testing"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestAgeMonthsAt(t *testing.T) {
	t.Parallel()

	birthday := time.Date(2018, 3, 15, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)
	if got := AgeMonthsAt(birthday, asOf); got != 71 {
		t.Fatalf("day-before anniversary = %d, want 71", got)
	}
	asOf = time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	if got := AgeMonthsAt(birthday, asOf); got != 72 {
		t.Fatalf("anniversary = %d, want 72", got)
	}
	if got := AgeMonthsAt(birthday, time.Time{}); got != 0 {
		t.Fatalf("zero asOf = %d, want 0", got)
	}
}

func TestBuildNormSubjectSnapshotUsesAssessmentAsOf(t *testing.T) {
	t.Parallel()

	birthday := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	snap := BuildNormSubjectSnapshot(&port.NormSubjectFacts{Gender: "female", Birthday: &birthday}, asOf)
	if snap.AgeMonths != 72 || snap.Gender != "female" {
		t.Fatalf("snapshot = %#v", snap)
	}
}

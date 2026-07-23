package evaluationinput

import (
	"context"
	"errors"
	"testing"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestAgeMonthsAt(t *testing.T) {
	t.Parallel()

	birthday := time.Date(2018, 3, 15, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)
	if got, ok := AgeMonthsAt(birthday, asOf); !ok || got != 71 {
		t.Fatalf("day-before anniversary = %d, %v; want 71, true", got, ok)
	}
	asOf = time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	if got, ok := AgeMonthsAt(birthday, asOf); !ok || got != 72 {
		t.Fatalf("anniversary = %d, %v; want 72, true", got, ok)
	}
	if got, ok := AgeMonthsAt(birthday, time.Time{}); ok || got != 0 {
		t.Fatalf("zero asOf = %d, %v; want 0, false", got, ok)
	}
	newborn := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	if got, ok := AgeMonthsAt(newborn, time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)); !ok || got != 0 {
		t.Fatalf("known newborn = %d, %v; want 0, true", got, ok)
	}
}

type failingNormSubjectReader struct {
	err error
}

func (s failingNormSubjectReader) ReadNormSubjectFacts(context.Context, uint64) (*port.NormSubjectFacts, error) {
	return nil, s.err
}

func TestResolveNormSubjectPreservesCanceledDependencyCause(t *testing.T) {
	t.Parallel()

	_, err := resolveNormSubject(
		context.Background(),
		failingNormSubjectReader{err: context.Canceled},
		port.InputRef{TesteeID: 7},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want wrapped context.Canceled", err)
	}
	var failure port.FailureKindCarrier
	if !errors.As(err, &failure) || failure.FailureKind() != port.FailureKindDependencyUnavailable {
		t.Fatalf("error = %T %v, want dependency_unavailable", err, err)
	}
	var retryable port.RetryableCarrier
	if !errors.As(err, &retryable) || !retryable.Retryable() {
		t.Fatalf("error = %T %v, want retryable", err, err)
	}
	var category port.DependencyCategoryCarrier
	if !errors.As(err, &category) || category.DependencyCategory() != port.DependencyCategoryActor {
		t.Fatalf("error = %T %v, want actor dependency", err, err)
	}
}

func TestBuildNormSubjectSnapshotUsesAssessmentAsOf(t *testing.T) {
	t.Parallel()

	birthday := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	snap := BuildNormSubjectSnapshot(&port.NormSubjectFacts{Gender: "female", Birthday: &birthday}, asOf)
	if snap.AgeMonths == nil || *snap.AgeMonths != 72 || snap.Gender != "female" {
		t.Fatalf("snapshot = %#v", snap)
	}
	missing := BuildNormSubjectSnapshot(&port.NormSubjectFacts{Gender: "female"}, asOf)
	if missing.AgeMonths != nil {
		t.Fatalf("missing birthday age = %#v, want nil", missing.AgeMonths)
	}
}

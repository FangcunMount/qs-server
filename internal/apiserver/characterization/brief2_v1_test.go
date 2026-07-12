package characterization_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
)

func TestV1Brief2ExecuteAppliesNormTScore(t *testing.T) {
	t.Parallel()

	a := draftBehavioralRatingAssessment(t)
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	a.ClearEvents()

	svc, capture := newV1RecordingExecuteService(t, a, &charInputResolver{snapshot: brief2InputSnapshot()})
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	result := capture.outcome.Execution
	if result == nil || len(result.Dimensions) == 0 {
		t.Fatal("expected assessment outcome with dimensions")
	}
	dim := result.Dimensions[0]
	if got := charDerivedScore(dim.DerivedScores, evaluationfact.ScoreKindTScore); got != 65 {
		t.Fatalf("t_score = %v, want 65", got)
	}
	if got := charDerivedScore(dim.DerivedScores, evaluationfact.ScoreKindPercentile); got != 90 {
		t.Fatalf("percentile = %v, want 90", got)
	}
	if dim.Level == nil || dim.Level.Code != "elevated" {
		t.Fatalf("dimension level = %#v, want elevated", dim.Level)
	}
}

func charDerivedScore(scores []evaluationfact.ScoreValue, kind evaluationfact.ScoreKind) float64 {
	for _, score := range scores {
		if score.Kind == kind {
			return score.Value
		}
	}
	return 0
}

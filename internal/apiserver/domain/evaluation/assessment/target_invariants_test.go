package assessment

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// These tests protect the accepted Evaluation lifecycle target during the
// remaining domain refactor batches.
func TestTargetEvaluatedAssessmentIsSuccessfulTerminalState(t *testing.T) {
	t.Run("status is terminal", func(t *testing.T) {
		a := targetEvaluatedAssessment(t)
		if !a.Status().IsTerminal() {
			t.Fatalf("evaluated status must be terminal, got %s", a.Status())
		}
	})

	t.Run("later module failure cannot rewrite evaluation facts", func(t *testing.T) {
		a := targetEvaluatedAssessment(t)
		beforeScore := *a.TotalScore()
		a.ClearEvents()

		if err := a.MarkAsFailed("report generation failed"); err == nil {
			t.Fatal("MarkAsFailed from evaluated must be rejected")
		}
		if !a.Status().IsEvaluated() {
			t.Fatalf("status = %s, want evaluated", a.Status())
		}
		if a.TotalScore() == nil || *a.TotalScore() != beforeScore {
			t.Fatalf("total score = %v, want preserved %v", a.TotalScore(), beforeScore)
		}
		if len(a.Events()) != 0 {
			t.Fatalf("events = %#v, later module failure must not emit assessment events", a.Events())
		}
	})
}

func targetEvaluatedAssessment(t *testing.T) *Assessment {
	t.Helper()
	a, err := NewAssessment(
		1,
		testee.NewID(1001),
		NewQuestionnaireRefByCode(meta.NewCode("Q-TARGET"), "1.0.0"),
		NewAnswerSheetRef(meta.FromUint64(2001)),
		NewAdhocOrigin(),
		WithID(NewID(5001)),
		WithEvaluationModel(NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("S-TARGET"), "1.0.0", "target scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	score := 12.0
	if err := a.ApplyScoringProjection(ScoringProjection{ModelRef: *a.EvaluationModelRef(), Summary: ResultSummary{PrimaryLabel: "evaluated"}, Score: &score}); err != nil {
		t.Fatalf("ApplyScoringProjection: %v", err)
	}
	return a
}

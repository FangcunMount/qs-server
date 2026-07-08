package scoring

import (
	"context"
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

func TestHandlerDelegatesToCalculationScoring(t *testing.T) {
	t.Parallel()

	result, err := NewDefaultHandler().Score(context.Background(), EvaluateInput{
		Scale: &scalesnapshot.ScaleSnapshot{
			Code: "S-001",
			Factors: []scalesnapshot.FactorSnapshot{
				{
					Code:            "total",
					Title:           "total",
					IsTotalScore:    true,
					QuestionCodes:   []string{"q1"},
					ScoringStrategy: string(ScoringStrategySum),
				},
			},
		},
		AnswerSheet: &evaluationinput.AnswerSheet{
			Answers: []evaluationinput.Answer{
				{QuestionCode: "q1", Score: 7},
			},
		},
	})
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}
	if result.TotalScore != 7 {
		t.Fatalf("total score = %.1f, want 7", result.TotalScore)
	}
}

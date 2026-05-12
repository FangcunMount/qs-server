package evaluation

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestExecutorConvertsSnapshotThroughScaleEvaluator(t *testing.T) {
	executor := NewExecutor(nil, nil)
	snapshot := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:    evaluationinput.EvaluationModelKindScale,
			Code:    "S-001",
			Version: "1.0.0",
			Title:   "Scale",
		},
		MedicalScale: &evaluationinput.ScaleSnapshot{
			Code: "S-001",
			Factors: []evaluationinput.FactorSnapshot{
				{
					Code:            "total",
					Title:           "total",
					IsTotalScore:    true,
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: "sum",
					InterpretRules: []evaluationinput.InterpretRuleSnapshot{
						{Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low", Suggestion: "keep"},
					},
				},
			},
		},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "q1", Score: 3},
				{QuestionCode: "q2", Score: 4},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{},
	}

	result, err := executor.EvaluateScale(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("EvaluateScale returned error: %v", err)
	}
	if result.ModelRef.Kind() != assessment.EvaluationModelKindScale || result.ModelRef.Code().String() != "S-001" {
		t.Fatalf("unexpected model ref: %#v", result.ModelRef)
	}
	if result.TotalScore != 7 || result.RiskLevel != assessment.RiskLevelLow {
		t.Fatalf("result summary = score %.1f risk %s, want 7/low", result.TotalScore, result.RiskLevel)
	}
}

func TestExecutorImplementsEvaluationExecutorContract(t *testing.T) {
	var _ interface {
		Kind() assessment.EvaluationModelKind
	} = (*Executor)(nil)
}

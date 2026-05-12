package evaluation

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestExecutorConvertsSnapshotThroughScaleEvaluator(t *testing.T) {
	executor := NewExecutor(nil)
	modelRef := assessment.NewEvaluationModelRefByCode(assessment.EvaluationModelKindScale, meta.NewCode("S-001"), "1.0.0", "Scale")
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)),
		assessment.NewAdhocOrigin(),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	snapshot := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:    evaluationinput.EvaluationModelKindScale,
			Code:    "S-001",
			Version: "1.0.0",
			Title:   "Scale",
		},
		MedicalScale: &evaluationinput.ScaleSnapshot{
			Code:              "S-001",
			QuestionnaireCode: "Q-001",
			Status:            "published",
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

	result, err := executor.EvaluateScale(context.Background(), a, snapshot)
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

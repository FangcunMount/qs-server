package evaluation

import (
	"context"
	"errors"
	"testing"

	evaluationengine "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/evaluation"
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

	result, err := executor.Execute(context.Background(), evaluationengine.ExecutionInput{
		Assessment: a,
		Input:      snapshot,
	})
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

func TestScaleEvaluationServiceOrchestratesDependencies(t *testing.T) {
	validator := &stubValidator{}
	assembler := &stubAssembler{
		output: scaleevaluation.ScaleEvaluationInput{
			Scale: scaleevaluation.ScaleEvaluationModel{
				Factors: []domainScale.FactorSnapshot{{Code: domainScale.NewFactorCode("f1"), IsTotalScore: true}},
			},
		},
	}
	evaluator := scaleevaluation.NewEvaluator(stubScoringRegistry{})
	mapper := &stubMapper{
		output: assessment.NewEvaluationResult(1, assessment.RiskLevelLow, "c", "s", nil),
	}
	service := NewService(validator, assembler, evaluator, mapper)

	a, _ := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)),
		assessment.NewAdhocOrigin(),
	)
	_ = a.Submit()
	snapshot := &evaluationinput.InputSnapshot{
		MedicalScale: &evaluationinput.ScaleSnapshot{
			QuestionnaireCode: "Q-001",
			Status:            "published",
			Factors: []evaluationinput.FactorSnapshot{
				{Code: "f1", IsTotalScore: true, ScoringStrategy: "sum"},
			},
		},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{},
	}
	result, err := service.Evaluate(context.Background(), a, snapshot)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !validator.called || !assembler.called || !mapper.called {
		t.Fatalf("expected validator/assembler/mapper to be called, got %v/%v/%v", validator.called, assembler.called, mapper.called)
	}
}

type stubValidator struct {
	called bool
	err    error
}

func (s *stubValidator) Validate(input ScaleExecutionInput) error {
	s.called = true
	return s.err
}

type stubAssembler struct {
	called bool
	output scaleevaluation.ScaleEvaluationInput
}

func (s *stubAssembler) FromSnapshot(_ *evaluationinput.InputSnapshot) scaleevaluation.ScaleEvaluationInput {
	s.called = true
	return s.output
}

type stubMapper struct {
	called bool
	output *assessment.EvaluationResult
}

func (s *stubMapper) ToEvaluationResult(
	_ *scaleevaluation.ScaleEvaluationResult,
	_ *assessment.Assessment,
	_ *evaluationinput.InputSnapshot,
) *assessment.EvaluationResult {
	s.called = true
	return s.output
}

type stubScoringRegistry struct{}

func (stubScoringRegistry) ScoreFactor(context.Context, domainScale.FactorSnapshot, []float64) (float64, error) {
	return 1, errors.New("ignore")
}

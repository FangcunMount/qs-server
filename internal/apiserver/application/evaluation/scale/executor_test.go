package scale

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scale"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scale"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/snapshot"
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
		MedicalScale: &scalesnapshot.ScaleSnapshot{
			Code:                 "S-001",
			ScaleVersion:         "1.0.0",
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Status:               "published",
			Factors: []scalesnapshot.FactorSnapshot{
				{
					Code:            "total",
					Title:           "total",
					IsTotalScore:    true,
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: "sum",
					InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
						{Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low", Suggestion: "keep"},
					},
				},
			},
		},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "q1", Score: 3},
				{QuestionCode: "q2", Score: 4},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-001", Version: "1.0.0"},
	}

	result, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
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

func TestInputValidatorRejectsQuestionnaireVersionMismatch(t *testing.T) {
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
	err = DefaultInputValidator{}.Validate(ScaleExecutionInput{
		Assessment: a,
		Input: &evaluationinput.InputSnapshot{
			Model:        evaluationinput.NewScaleModelSnapshot(&scalesnapshot.ScaleSnapshot{Code: "S-001", ScaleVersion: "1.0.0", Title: "Scale"}),
			ModelPayload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "S-001", ScaleVersion: "1.0.0", QuestionnaireCode: "Q-001", QuestionnaireVersion: "2.0.0", Status: "published", Factors: []scalesnapshot.FactorSnapshot{{Code: "total"}}}},
			AnswerSheet:  &evaluationinput.AnswerSheetSnapshot{ID: 1, QuestionnaireCode: "Q-001", QuestionnaireVersion: "1.0.0"},
			Questionnaire: &evaluationinput.QuestionnaireSnapshot{
				Code:    "Q-001",
				Version: "1.0.0",
			},
		},
	})
	if err == nil {
		t.Fatal("Validate error = nil, want questionnaire version mismatch")
	}
}

func TestScaleInterpretationServiceOrchestratesDependencies(t *testing.T) {
	validator := &stubValidator{}
	mapper := &stubMapper{
		output: assessment.NewEvaluationResult(1, assessment.RiskLevelLow, "c", "s", nil),
	}
	service := NewService(validator, evaluationscale.NewHandler(stubScoringRegistry{}), mapper)

	a, _ := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)),
		assessment.NewAdhocOrigin(),
	)
	_ = a.Submit()
	snapshot := &evaluationinput.InputSnapshot{
		MedicalScale: &scalesnapshot.ScaleSnapshot{
			Code:                 "S-001",
			ScaleVersion:         "1.0.0",
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Status:               "published",
			Factors: []scalesnapshot.FactorSnapshot{
				{
					Code:            "f1",
					IsTotalScore:    true,
					ScoringStrategy: "sum",
					QuestionCodes:   []string{"f1"},
					InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
						{Min: 0, Max: 10, RiskLevel: "low", Conclusion: "c", Suggestion: "s"},
					},
				},
			},
		},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "f1", Score: 1},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{
			Code:    "Q-001",
			Version: "1.0.0",
			Questions: []evaluationinput.QuestionSnapshot{
				{Code: "f1"},
			},
		},
	}
	result, err := service.Evaluate(context.Background(), a, snapshot)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !validator.called || !mapper.called {
		t.Fatalf("expected validator/mapper to be called, got %v/%v", validator.called, mapper.called)
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

type stubMapper struct {
	called bool
	output *assessment.EvaluationResult
}

func (s *stubMapper) ToEvaluationResult(
	_ *scaleinterpretation.ScaleInterpretationResult,
	_ *assessment.Assessment,
	_ *evaluationinput.InputSnapshot,
) *assessment.EvaluationResult {
	s.called = true
	return s.output
}

type stubScoringRegistry struct{}

func (stubScoringRegistry) ScoreFactor(context.Context, scalesnapshot.FactorSnapshot, []float64) (float64, error) {
	return 1, nil
}

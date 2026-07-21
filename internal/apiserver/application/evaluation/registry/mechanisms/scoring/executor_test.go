package scoring

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
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
		DefinitionV2: scaleDefinition("total", []string{"q1", "q2"}),
		Model: &evaluationinput.ModelSnapshot{
			Kind:    evaluationinput.EvaluationModelKindScale,
			Code:    "S-001",
			Version: "1.0.0",
			Title:   "Scale",
		},
		ModelPayload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{
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
		}},
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
	if result.Primary == nil || result.Primary.Value != 7 || result.Level == nil || result.Level.Code != string(assessment.RiskLevelLow) {
		t.Fatalf("result summary = primary %#v level %#v, want 7/low", result.Primary, result.Level)
	}
}

func TestExecutorImplementsEvaluationExecutorContract(t *testing.T) {
	var _ interface {
		Key() evaluation.ExecutionIdentity
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
	err = DefaultInputValidator{}.Validate(ExecutionInput{
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

func TestExecutorOrchestratesValidatorAndHandler(t *testing.T) {
	validator := &stubValidator{}
	handler := calcscoring.NewEvaluator(stubScoringRegistry{})
	executor := NewExecutorWithDeps(validator, handler)

	a, _ := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)),
		assessment.NewAdhocOrigin(),
	)
	_ = a.Submit()
	snapshot := &evaluationinput.InputSnapshot{
		DefinitionV2: scaleDefinition("f1", []string{"f1"}),
		ModelPayload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{
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
		}},
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
	result, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: a,
		Input:      snapshot,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected outcome, got nil")
	}
	if !validator.called {
		t.Fatal("expected validator to be called")
	}
	if result.Primary == nil || result.Primary.Kind != domainoutcome.ScoreKindRawTotal || result.Primary.Value != 1 {
		t.Fatalf("primary = %#v, want raw_total 1", result.Primary)
	}
	if result.Level == nil || result.Level.Code != string(assessment.RiskLevelLow) {
		t.Fatalf("level = %#v, want low", result.Level)
	}
}

func scaleDefinition(factorCode string, questionCodes []string) *definition.Definition {
	sources := make([]factor.ScoringSource, 0, len(questionCodes))
	for _, code := range questionCodes {
		sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: code, ScoringMode: factor.QuestionScoringModeQuestionScore})
	}
	return &definition.Definition{
		Measure: definition.MeasureSpec{
			Factors:     []factor.Factor{{Code: factorCode, Title: factorCode, Role: factor.FactorRoleTotal}},
			FactorGraph: factor.FactorGraph{Roots: []string{factorCode}},
			Scoring:     []factor.Scoring{{FactorCode: factorCode, Strategy: factor.ScoringStrategySum, Sources: sources}},
		},
		Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: factorCode,
			Rules: []conclusion.ScoreRangeOutcome{{
				MinScore: 0, MaxScore: 10, OutcomeCode: "low",
			}},
		}},
	}
}

type stubValidator struct {
	called bool
	err    error
}

func (s *stubValidator) Validate(input ExecutionInput) error {
	s.called = true
	return s.err
}

type stubScoringRegistry struct{}

func (stubScoringRegistry) ScoreFactor(context.Context, calcscoring.Factor, []float64) (float64, error) {
	return 1, nil
}

package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	typologyapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// V1 contract: SBTI DRUNK hidden outcome flows through executor → report with special fields.
func TestV1SBTIDrunkExecutorToReportPreservesSpecialFields(t *testing.T) {
	model := sbtiSpecialTestModel()
	payload := modeltypology.FromSBTI(model)

	executor, err := typologyapp.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	result, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: submittedSBTIAssessment(t),
		Input: sbtiSpecialInputSnapshot(payload, []port.AnswerSnapshot{
			{QuestionCode: "drink_gate_q2", Value: "C"},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail := requirePersonalityTypeDetail(t, result.Detail.Payload)
	if detail.TypeCode != "DRUNK" {
		t.Fatalf("TypeCode = %s, want DRUNK", detail.TypeCode)
	}
	if detail.SpecialTrigger != "hidden:drink_gate_q2_answer=2" {
		t.Fatalf("SpecialTrigger = %q, want hidden trigger", detail.SpecialTrigger)
	}

	report, err := mustConfiguredReportBuilder(t).Build(context.Background(), evaloutcome.NewOutcomeFromLegacyResult(
		submittedSBTIAssessment(t), nil,
		assessment.NewModelEvaluationResult(
			*submittedSBTIAssessment(t).EvaluationModelRef(),
			assessment.ResultSummary{PrimaryLabel: detail.TypeCode},
			assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindPersonality, Payload: detail},
		),
	))
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	extra := report.ModelExtra()
	if extra == nil || !extra.IsSpecial || extra.SpecialTrigger != "hidden:drink_gate_q2_answer=2" {
		t.Fatalf("ModelExtra = %#v, want special DRUNK", extra)
	}
	if extra.TypeCode != "DRUNK" {
		t.Fatalf("TypeCode = %s, want DRUNK", extra.TypeCode)
	}
}

// V1 contract: SBTI HHHH fallback flows through executor → report with fallback trigger.
func TestV1SBTIFallbackExecutorToReportPreservesSpecialFields(t *testing.T) {
	model := sbtiSpecialTestModel()
	model.FallbackSimilarityThreshold = 0.9
	payload := modeltypology.FromSBTI(model)

	executor, err := typologyapp.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	result, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: submittedSBTIAssessment(t),
		Input: sbtiSpecialInputSnapshot(payload, []port.AnswerSnapshot{
			{QuestionCode: "Q1", Value: "A"},
			{QuestionCode: "Q2", Value: "A"},
			{QuestionCode: "Q3", Value: "A"},
			{QuestionCode: "Q4", Value: "A"},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail := requirePersonalityTypeDetail(t, result.Detail.Payload)
	if detail.TypeCode != "HHHH" {
		t.Fatalf("TypeCode = %s, want HHHH", detail.TypeCode)
	}
	if detail.SpecialTrigger != "fallback:best_match<60%" {
		t.Fatalf("SpecialTrigger = %q, want fallback trigger", detail.SpecialTrigger)
	}

	report, err := mustConfiguredReportBuilder(t).Build(context.Background(), evaloutcome.NewOutcomeFromLegacyResult(
		submittedSBTIAssessment(t), nil,
		assessment.NewModelEvaluationResult(
			*submittedSBTIAssessment(t).EvaluationModelRef(),
			assessment.ResultSummary{PrimaryLabel: detail.TypeCode},
			assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindPersonality, Payload: detail},
		),
	))
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	extra := report.ModelExtra()
	if extra == nil || !extra.IsSpecial || extra.SpecialTrigger != "fallback:best_match<60%" {
		t.Fatalf("ModelExtra = %#v, want special HHHH fallback", extra)
	}
	if extra.TypeCode != "HHHH" {
		t.Fatalf("TypeCode = %s, want HHHH", extra.TypeCode)
	}
}

func sbtiSpecialTestModel() *modeltypology.SBTILegacyModel {
	return &modeltypology.SBTILegacyModel{
		Code:                        "SBTI_FUN",
		Version:                     "1.0.0",
		Title:                       "SBTI",
		QuestionnaireCode:           "SBTI_FUN",
		QuestionnaireVersion:        "1.0.0",
		Status:                      "published",
		FallbackSimilarityThreshold: 0.6,
		DimensionOrder:              []string{"D1", "D2"},
		Dimensions: map[string]modeltypology.SBTILegacyDimension{
			"D1": {Code: "D1", Name: "D1", Model: "M1"},
			"D2": {Code: "D2", Name: "D2", Model: "M2"},
		},
		QuestionMappings: []modeltypology.SBTILegacyQuestionMapping{
			{QuestionCode: "Q1", Dimension: "D1", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q2", Dimension: "D1", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q3", Dimension: "D2", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q4", Dimension: "D2", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
		},
		NormalOutcomes: []modeltypology.SBTILegacyOutcome{
			{Code: "HIGH", Name: "高能者", Pattern: "HH", OneLiner: "all high"},
		},
		SpecialOutcomes: []modeltypology.SBTILegacyOutcome{
			{Code: "HHHH", Name: "傻乐者", Trigger: "fallback:best_match<60%", IsSpecial: true},
			{Code: "DRUNK", Name: "酒鬼", Trigger: "hidden:drink_gate_q2_answer=2", IsSpecial: true},
		},
		DrinkTrigger: modeltypology.SBTILegacyDrinkTrigger{
			QuestionCodes: []string{"drink_gate_q2"},
			OptionValues:  []string{"C"},
		},
	}
}

func sbtiSpecialInputSnapshot(payload *modeltypology.Payload, answers []port.AnswerSnapshot) *port.InputSnapshot {
	return &port.InputSnapshot{
		Model:        port.NewTypologyModelSnapshot(payload),
		ModelPayload: port.TypologyModelPayload{Payload: payload},
		AnswerSheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    payload.QuestionnaireCode,
			QuestionnaireVersion: payload.QuestionnaireVersion,
			Answers:              answers,
		},
		Questionnaire: &port.QuestionnaireSnapshot{
			Code:    payload.QuestionnaireCode,
			Version: payload.QuestionnaireVersion,
		},
	}
}

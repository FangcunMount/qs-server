package typology

import (
	"testing"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestScoreSBTIMatchesClosestOutcome(t *testing.T) {
	model := sbtiScorerTestModel()
	sheet := &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
		{QuestionCode: "Q1", Value: "C"},
		{QuestionCode: "Q2", Value: "C"},
		{QuestionCode: "Q3", Value: "C"},
		{QuestionCode: "Q4", Value: "C"},
	}}

	got, err := ScoreSBTI(model, sheet)
	if err != nil {
		t.Fatalf("ScoreSBTI returned error: %v", err)
	}
	if got.TypeCode != "HIGH" {
		t.Fatalf("TypeCode = %s, want HIGH", got.TypeCode)
	}
	if got.Similarity != 1 {
		t.Fatalf("Similarity = %.2f, want 1", got.Similarity)
	}
	if got.Dimensions[0].Level != "H" || got.Dimensions[1].Level != "H" {
		t.Fatalf("dimension levels = %#v, want both H", got.Dimensions)
	}
}

func TestScoreSBTIUsesFallbackWhenBestSimilarityIsLow(t *testing.T) {
	model := sbtiScorerTestModel()
	model.FallbackSimilarityThreshold = 0.9
	sheet := &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
		{QuestionCode: "Q1", Value: "A"},
		{QuestionCode: "Q2", Value: "A"},
		{QuestionCode: "Q3", Value: "A"},
		{QuestionCode: "Q4", Value: "A"},
	}}

	got, err := ScoreSBTI(model, sheet)
	if err != nil {
		t.Fatalf("ScoreSBTI returned error: %v", err)
	}
	if got.TypeCode != "HHHH" {
		t.Fatalf("TypeCode = %s, want HHHH", got.TypeCode)
	}
	if got.SpecialTrigger != "fallback:best_match<60%" {
		t.Fatalf("SpecialTrigger = %s, want fallback trigger", got.SpecialTrigger)
	}
}

func TestScoreSBTIUsesDrinkHiddenOutcome(t *testing.T) {
	model := sbtiScorerTestModel()
	sheet := &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
		{QuestionCode: "drink_gate_q2", Value: "C"},
	}}

	got, err := ScoreSBTI(model, sheet)
	if err != nil {
		t.Fatalf("ScoreSBTI returned error: %v", err)
	}
	if got.TypeCode != "DRUNK" {
		t.Fatalf("TypeCode = %s, want DRUNK", got.TypeCode)
	}
	if got.Similarity != 1 {
		t.Fatalf("Similarity = %.2f, want 1", got.Similarity)
	}
}

func sbtiScorerTestModel() *modeltypology.SBTILegacyModel {
	return &modeltypology.SBTILegacyModel{
		Code:                        "SBTI_FUN",
		Version:                     "1.0.0",
		Title:                       "SBTI",
		QuestionnaireCode:           "SBTI_FUN",
		QuestionnaireVersion:        "1.0.0",
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
			{Code: "MID", Name: "中间者", Pattern: "MM", OneLiner: "all mid"},
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

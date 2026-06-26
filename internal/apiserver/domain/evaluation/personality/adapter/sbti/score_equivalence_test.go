package sbti_test

import (
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	sbtiadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/sbti"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

func TestScoreMatchesLegacyScorerForUnitModel(t *testing.T) {
	model := sbtiScorerTestModel()
	cases := []struct {
		name  string
		sheet *evaluationinput.AnswerSheet
	}{
		{
			name: "closest_outcome",
			sheet: &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
				{QuestionCode: "Q1", Value: "C"},
				{QuestionCode: "Q2", Value: "C"},
				{QuestionCode: "Q3", Value: "C"},
				{QuestionCode: "Q4", Value: "C"},
			}},
		},
		{
			name: "fallback",
			sheet: &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
				{QuestionCode: "Q1", Value: "A"},
				{QuestionCode: "Q2", Value: "A"},
				{QuestionCode: "Q3", Value: "A"},
				{QuestionCode: "Q4", Value: "A"},
			}},
		},
		{
			name: "drink_hidden",
			sheet: &evaluationinput.AnswerSheet{Answers: []evaluationinput.Answer{
				{QuestionCode: "drink_gate_q2", Value: "C"},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testModel := *model
			if tc.name == "fallback" {
				testModel.FallbackSimilarityThreshold = 0.9
			}
			legacy, err := evaluationtypology.ScoreSBTIReference(&testModel, tc.sheet)
			if err != nil {
				t.Fatalf("ScoreSBTI: %v", err)
			}
			got, err := sbtiadapter.Score(&testModel, tc.sheet)
			if err != nil {
				t.Fatalf("adapter Score: %v", err)
			}
			assertSBTIResultEqual(t, legacy, got)
		})
	}
}

func assertSBTIResultEqual(t *testing.T, want, got evaluationtypology.SBTIResultDetail) {
	t.Helper()
	if got.TypeCode != want.TypeCode {
		t.Fatalf("TypeCode = %s, want %s", got.TypeCode, want.TypeCode)
	}
	if got.TypeName != want.TypeName {
		t.Fatalf("TypeName = %s, want %s", got.TypeName, want.TypeName)
	}
	if got.OneLiner != want.OneLiner {
		t.Fatalf("OneLiner = %s, want %s", got.OneLiner, want.OneLiner)
	}
	if got.Pattern != want.Pattern {
		t.Fatalf("Pattern = %s, want %s", got.Pattern, want.Pattern)
	}
	if got.Similarity != want.Similarity {
		t.Fatalf("Similarity = %.4f, want %.4f", got.Similarity, want.Similarity)
	}
	if got.ImageURL != want.ImageURL {
		t.Fatalf("ImageURL = %s, want %s", got.ImageURL, want.ImageURL)
	}
	if got.SpecialTrigger != want.SpecialTrigger {
		t.Fatalf("SpecialTrigger = %s, want %s", got.SpecialTrigger, want.SpecialTrigger)
	}
	if len(got.Dimensions) != len(want.Dimensions) {
		t.Fatalf("dimensions = %d, want %d", len(got.Dimensions), len(want.Dimensions))
	}
	for i := range want.Dimensions {
		if got.Dimensions[i] != want.Dimensions[i] {
			t.Fatalf("dimension[%d] = %#v, want %#v", i, got.Dimensions[i], want.Dimensions[i])
		}
	}
	if got.Outcome.Code != want.Outcome.Code {
		t.Fatalf("outcome code = %s, want %s", got.Outcome.Code, want.Outcome.Code)
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

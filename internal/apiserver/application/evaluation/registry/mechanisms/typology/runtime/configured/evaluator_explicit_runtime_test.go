package configured_test

import (
	"testing"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func TestEvaluatorScoresExplicitRuntimeWithoutAlgorithm(t *testing.T) {
	payload := explicitPoleCompositionPayload()
	payload.Algorithm = ""

	got, err := configured.NewEvaluator().Score(payload, explicitPoleCompositionSheet())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	detail, err := outcometypology.PersonalityTypeDetailFromPayload(got.Detail)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.TypeCode != "INTJ" {
		t.Fatalf("TypeCode = %s, want INTJ", detail.TypeCode)
	}
}

func TestEvaluatorAppliesGenericFallbackSpecialRule(t *testing.T) {
	payload := explicitNearestPatternPayload()
	got, err := configured.NewEvaluator().Score(payload, explicitNearestPatternLowSheet())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	detail, err := outcometypology.PersonalityTypeDetailFromPayload(got.Detail)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.TypeCode != "LOW_MATCH" {
		t.Fatalf("TypeCode = %s, want LOW_MATCH", detail.TypeCode)
	}
	if got.SpecialMatch == nil || got.SpecialMatch.OutcomeCode != "LOW_MATCH" {
		t.Fatalf("SpecialMatch = %#v, want LOW_MATCH", got.SpecialMatch)
	}
}

func explicitPoleCompositionPayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:                 "CUSTOM_POLE_V1",
		Version:              "1.0.0",
		QuestionnaireCode:    "CUSTOM_POLE_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		DimensionOrder:       []string{"EI", "SN", "TF", "JP"},
		Dimensions: map[string]modeltypology.Dimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
			"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
			"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
			"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
		},
		Outcomes: []modeltypology.Outcome{
			{Code: "INTJ", Name: "建筑师", OneLiner: "独立战略家"},
		},
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				DimensionOrder: []string{"EI", "SN", "TF", "JP"},
				Dimensions: map[string]modeltypology.Dimension{
					"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
					"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
					"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
					"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
				},
				QuestionMappings: []modeltypology.QuestionMapping{
					{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
					{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
					{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
					{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
				},
			},
			Decision: modeltypology.PersonalityDecisionSpec{
				Kind: "pole_composition",
			},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{
				DetailKind: modeltypology.OutcomeDetailPersonalityType,
			},
			Report: modeltypology.ReportSpec{
				Kind:          modeltypology.ReportKindPersonalityType,
				CategoryLabel: "Custom Pole Model",
			},
		},
	}
}

func explicitPoleCompositionSheet() *evalinput.AnswerSheet {
	return &evalinput.AnswerSheet{Answers: []evalinput.Answer{
		{QuestionCode: "Q_EI", Score: 1},
		{QuestionCode: "Q_SN", Score: 5},
		{QuestionCode: "Q_TF", Score: 1},
		{QuestionCode: "Q_JP", Score: 1},
	}}
}

func explicitNearestPatternPayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:    "CUSTOM_PATTERN_V1",
		Version: "1.0.0",
		Status:  "published",
		Outcomes: []modeltypology.Outcome{
			{Code: "HIGH", Name: "High", Pattern: "HH"},
			{Code: "LOW_MATCH", Name: "Low Match", IsSpecial: true, Trigger: "fallback:best_match<90%"},
		},
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				Factors: map[string]modeltypology.FactorSpec{
					"D1": {
						ID:   "D1",
						Code: "D1",
						Name: "Dimension 1",
						Kind: modeltypology.FactorSpecKindLeaf,
						Contributions: []modeltypology.FactorContributionSpec{
							{QuestionCode: "Q1", OptionScores: map[string]float64{"A": 1, "C": 6}},
						},
						OptionScoring: modeltypology.FactorOptionScoringStrict,
					},
					"D2": {
						ID:   "D2",
						Code: "D2",
						Name: "Dimension 2",
						Kind: modeltypology.FactorSpecKindLeaf,
						Contributions: []modeltypology.FactorContributionSpec{
							{QuestionCode: "Q2", OptionScores: map[string]float64{"A": 1, "C": 6}},
						},
						OptionScoring: modeltypology.FactorOptionScoringStrict,
					},
				},
				Roots: []string{"D1", "D2"},
				Dimensions: map[string]modeltypology.Dimension{
					"D1": {Code: "D1", Name: "Dimension 1"},
					"D2": {Code: "D2", Name: "Dimension 2"},
				},
			},
			Decision: modeltypology.PersonalityDecisionSpec{
				Kind:                        "nearest_pattern",
				FallbackSimilarityThreshold: 0.9,
				FallbackCode:                "LOW_MATCH",
				LevelRule:                   &modeltypology.LevelRuleSpec{LowMax: 3, HighMin: 5},
			},
			SpecialRules: []modeltypology.SpecialRuleSpec{
				{
					Code:        "fallback:LOW_MATCH",
					Kind:        modeltypology.SpecialRuleKindFallbackThreshold,
					Phase:       modeltypology.SpecialRuleAfterDecision,
					OutcomeCode: "LOW_MATCH",
				},
			},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{
				DetailKind: modeltypology.OutcomeDetailPersonalityType,
			},
			Report: modeltypology.ReportSpec{
				Kind: modeltypology.ReportKindPersonalityType,
			},
		},
	}
}

func explicitNearestPatternLowSheet() *evalinput.AnswerSheet {
	return &evalinput.AnswerSheet{Answers: []evalinput.Answer{
		{QuestionCode: "Q1", Value: "A"},
		{QuestionCode: "Q2", Value: "C"},
	}}
}

package configured_test

import (
	"testing"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func TestEvaluatorCompositeFactorAggregatesBeforePoleDecision(t *testing.T) {
	payload := compositePolePayload()
	got, err := configured.NewEvaluator().Score(payload, compositePoleSheet())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	detail, err := outcometypology.PersonalityTypeDetailFromPayload(got.Detail)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.TypeCode != "R" {
		t.Fatalf("TypeCode = %s, want R from composite sum above threshold", detail.TypeCode)
	}
	if len(detail.Dimensions) != 1 || detail.Dimensions[0].RawScore != 26 {
		t.Fatalf("dimensions = %#v, want composite raw 26", detail.Dimensions)
	}
}

func TestEvaluatorCompositeFactorMatchesFlatTraitProfileScores(t *testing.T) {
	explicit := compositeTraitPayload()
	flat := flatTraitPayload()

	explicitResult, err := configured.NewEvaluator().Score(explicit, compositeTraitSheet())
	if err != nil {
		t.Fatalf("explicit Score: %v", err)
	}
	flatResult, err := configured.NewEvaluator().Score(flat, compositeTraitSheet())
	if err != nil {
		t.Fatalf("flat Score: %v", err)
	}

	explicitDetail, err := outcometypology.TraitProfileDetailFromPayload(explicitResult.Detail)
	if err != nil {
		t.Fatalf("explicit detail: %v", err)
	}
	flatDetail, err := outcometypology.TraitProfileDetailFromPayload(flatResult.Detail)
	if err != nil {
		t.Fatalf("flat detail: %v", err)
	}
	if len(explicitDetail.Traits) != 2 || len(flatDetail.Traits) != 2 {
		t.Fatalf("traits = explicit:%d flat:%d", len(explicitDetail.Traits), len(flatDetail.Traits))
	}
	if explicitDetail.Traits[0].RawScore != flatDetail.Traits[0].RawScore {
		t.Fatalf("O raw = %.2f, want flat %.2f", explicitDetail.Traits[0].RawScore, flatDetail.Traits[0].RawScore)
	}
	if explicitDetail.Traits[1].RawScore != flatDetail.Traits[1].RawScore {
		t.Fatalf("C raw = %.2f, want flat %.2f", explicitDetail.Traits[1].RawScore, flatDetail.Traits[1].RawScore)
	}
}

func compositePolePayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:    "COMPOSITE_POLE_V1",
		Version: "1.0.0",
		Status:  "published",
		Outcomes: []modeltypology.Outcome{
			{Code: "R", Name: "Right"},
			{Code: "L", Name: "Left"},
		},
		Dimensions: map[string]modeltypology.Dimension{
			"TOTAL": {Code: "TOTAL", Name: "Total", LeftPole: "L", RightPole: "R", Threshold: 24},
		},
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				Factors: map[string]modeltypology.FactorSpec{
					"D1": {
						ID:       "D1",
						Kind:     modeltypology.FactorSpecKindLeaf,
						Constant: 10,
						Contributions: []modeltypology.FactorContributionSpec{
							{QuestionCode: "Q1", Sign: 1},
						},
					},
					"D2": {
						ID:       "D2",
						Kind:     modeltypology.FactorSpecKindLeaf,
						Constant: 10,
						Contributions: []modeltypology.FactorContributionSpec{
							{QuestionCode: "Q2", Sign: 1},
						},
					},
					"TOTAL": {
						ID:          "TOTAL",
						Kind:        modeltypology.FactorSpecKindComposite,
						Children:    []string{"D1", "D2"},
						Aggregation: modeltypology.FactorAggregationSum,
					},
				},
				Roots: []string{"TOTAL"},
				Dimensions: map[string]modeltypology.Dimension{
					"TOTAL": {Code: "TOTAL", Name: "Total", LeftPole: "L", RightPole: "R", Threshold: 24},
				},
			},
			Decision: modeltypology.PersonalityDecisionSpec{
				Kind: modelcatalog.DecisionKindPoleComposition,
			},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{
				DetailKind: modeltypology.OutcomeDetailPersonalityType,
			},
			Report: modeltypology.ReportSpec{
				Kind:          modeltypology.ReportKindPersonalityType,
				CategoryLabel: "Composite Pole",
			},
		},
	}
}

func compositeTraitPayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:    "COMPOSITE_TRAIT_V1",
		Version: "1.0.0",
		Status:  "published",
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				Factors: map[string]modeltypology.FactorSpec{
					"O1": {
						ID:   "O1",
						Code: "O",
						Name: "Openness-1",
						Kind: modeltypology.FactorSpecKindLeaf,
						Contributions: []modeltypology.FactorContributionSpec{
							{QuestionCode: "O1", Sign: 1},
						},
					},
					"O2": {
						ID:   "O2",
						Code: "O",
						Name: "Openness-2",
						Kind: modeltypology.FactorSpecKindLeaf,
						Contributions: []modeltypology.FactorContributionSpec{
							{QuestionCode: "O2", Sign: 1},
						},
					},
					"O": {
						ID:          "O",
						Code:        "O",
						Name:        "Openness",
						Kind:        modeltypology.FactorSpecKindComposite,
						Children:    []string{"O1", "O2"},
						Aggregation: modeltypology.FactorAggregationSum,
					},
					"C": {
						ID:   "C",
						Code: "C",
						Name: "Conscientiousness",
						Kind: modeltypology.FactorSpecKindLeaf,
						Contributions: []modeltypology.FactorContributionSpec{
							{QuestionCode: "C1", Sign: 1},
						},
					},
				},
				Roots: []string{"O", "C"},
				Dimensions: map[string]modeltypology.Dimension{
					"O": {Code: "O", Name: "Openness"},
					"C": {Code: "C", Name: "Conscientiousness"},
				},
			},
			Decision: modeltypology.PersonalityDecisionSpec{
				Kind: modelcatalog.DecisionKindTraitProfile,
			},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{
				DetailKind: modeltypology.OutcomeDetailTraitProfile,
			},
			Report: modeltypology.ReportSpec{
				Kind:          modeltypology.ReportKindTraitProfile,
				CategoryLabel: "Composite Trait",
			},
		},
	}
}

func flatTraitPayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:           "FLAT_TRAIT_V1",
		Version:        "1.0.0",
		Status:         "published",
		Algorithm:      modelcatalog.AlgorithmBigFive,
		DimensionOrder: []string{"O", "C"},
		Dimensions: map[string]modeltypology.Dimension{
			"O": {Code: "O", Name: "Openness"},
			"C": {Code: "C", Name: "Conscientiousness"},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{QuestionCode: "O1", Dimension: "O", Sign: 1},
			{QuestionCode: "O2", Dimension: "O", Sign: 1},
			{QuestionCode: "C1", Dimension: "C", Sign: 1},
		},
		MatchingSpec: modeltypology.MatchingSpec{
			Kind: modelcatalog.DecisionKindTraitProfile,
		},
	}
}

func compositePoleSheet() *evalinput.AnswerSheet {
	return &evalinput.AnswerSheet{Answers: []evalinput.Answer{
		{QuestionCode: "Q1", Score: 3},
		{QuestionCode: "Q2", Score: 3},
	}}
}

func compositeTraitSheet() *evalinput.AnswerSheet {
	return &evalinput.AnswerSheet{Answers: []evalinput.Answer{
		{QuestionCode: "O1", Score: 4},
		{QuestionCode: "O2", Score: 2},
		{QuestionCode: "C1", Score: 5},
	}}
}

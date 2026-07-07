package typology_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestCanonicalFactorsFromLegacyMBTIGraph(t *testing.T) {
	t.Parallel()

	payload := typology.FromMBTI(&typology.MBTILegacyModel{
		Code:           "MBTI_TEST",
		Version:        "1.0.0",
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]typology.MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"},
		},
		QuestionMappings: []typology.MBTILegacyQuestionMapping{
			{QuestionCode: "q1", Dimension: "EI", Sign: -1},
		},
	})
	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}

	factors := typology.CanonicalFactorsFromGraph(spec.FactorGraph)
	if len(factors) != 1 {
		t.Fatalf("factors = %#v", factors)
	}
	if factors[0].Code != "EI" || factors[0].ResolvedRole() != factor.FactorRoleDimension {
		t.Fatalf("factor = %#v", factors[0])
	}
	if len(factors[0].QuestionCodes) != 1 || factors[0].QuestionCodes[0] != "q1" {
		t.Fatalf("question codes = %#v", factors[0].QuestionCodes)
	}
	if factors[0].Classification == nil ||
		factors[0].Classification.NegativePole != "I" ||
		factors[0].Classification.PositivePole != "E" {
		t.Fatalf("classification = %#v", factors[0].Classification)
	}
}

func TestPayloadCanonicalFactorsUsesRuntimeSpec(t *testing.T) {
	t.Parallel()

	payload := &typology.Payload{
		Code:           "CUSTOM_V1",
		Version:        "1.0.0",
		DimensionOrder: []string{"O"},
		Dimensions: map[string]typology.Dimension{
			"O": {Code: "O", Name: "Openness"},
		},
		Runtime: &typology.RuntimeSpec{
			FactorGraph: typology.FactorGraphSpec{
				Factors: map[string]typology.FactorSpec{
					"openness": {
						ID:   "openness",
						Code: "O",
						Name: "Openness",
						Kind: typology.FactorSpecKindLeaf,
						Contributions: []typology.FactorContributionSpec{{
							QuestionCode: "q1",
						}},
					},
				},
				Roots: []string{"openness"},
			},
			Decision: typology.PersonalityDecisionSpec{Kind: modelcatalog.DecisionKindTraitProfile},
			OutcomeMapping: typology.OutcomeMappingSpec{
				DetailKind: typology.OutcomeDetailTraitProfile,
			},
			Report: typology.ReportSpec{Kind: typology.ReportKindTraitProfile},
		},
	}

	factors, err := payload.CanonicalFactors()
	if err != nil {
		t.Fatalf("CanonicalFactors: %v", err)
	}
	if len(factors) != 1 || factors[0].Code != "O" || factors[0].QuestionCodes[0] != "q1" {
		t.Fatalf("factors = %#v", factors)
	}
}

func TestCanonicalFactorsSkipsCompositeNodes(t *testing.T) {
	t.Parallel()

	factors := typology.CanonicalFactorsFromGraph(typology.FactorGraphSpec{
		Factors: map[string]typology.FactorSpec{
			"root": {
				ID:       "root",
				Kind:     typology.FactorSpecKindComposite,
				Children: []string{"leaf"},
			},
			"leaf": {
				ID:   "leaf",
				Code: "L1",
				Kind: typology.FactorSpecKindLeaf,
				Contributions: []typology.FactorContributionSpec{{
					QuestionCode: "q1",
				}},
			},
		},
		Roots: []string{"root"},
	})
	if len(factors) != 1 || factors[0].Code != "L1" {
		t.Fatalf("factors = %#v", factors)
	}
}

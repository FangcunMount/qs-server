package typology_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
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

	measure := spec.CanonicalMeasureSpec()
	factors := measure.Factors
	if len(factors) != 1 {
		t.Fatalf("factors = %#v", factors)
	}
	if factors[0].Code != "EI" || factors[0].ResolvedRole() != factor.FactorRoleDimension {
		t.Fatalf("factor = %#v", factors[0])
	}
	if len(measure.Scoring) != 1 ||
		measure.Scoring[0].Sources[0].Kind != factor.ScoringSourceQuestion ||
		measure.Scoring[0].Sources[0].Code != "q1" {
		t.Fatalf("scoring = %#v", measure.Scoring)
	}
}

func TestRuntimeSpecCanonicalMeasureSpecUsesExplicitGraph(t *testing.T) {
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

	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}
	measure := spec.CanonicalMeasureSpec()
	if len(measure.Factors) != 1 || measure.Factors[0].Code != "O" {
		t.Fatalf("factors = %#v", measure.Factors)
	}
	if len(measure.Scoring) != 1 || measure.Scoring[0].Sources[0].Code != "q1" {
		t.Fatalf("scoring = %#v", measure.Scoring)
	}
}

func TestCanonicalFactorsSkipsCompositeNodes(t *testing.T) {
	t.Parallel()

	measure := (&typology.RuntimeSpec{FactorGraph: typology.FactorGraphSpec{
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
	}}).CanonicalMeasureSpec()
	factors := measure.Factors
	if len(factors) != 1 || factors[0].Code != "L1" {
		t.Fatalf("factors = %#v", factors)
	}
}

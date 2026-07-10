package shared_test

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	sharedpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
)

func TestDefinitionBodyJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := sharedpayload.DefinitionBody{
		Dimensions: []sharedpayload.DimensionRule{{
			Code: "total", Title: "总分", ScoringStrategy: "sum", IsTotalScore: true,
			ScoringParams: &sharedpayload.ScoringParamsPayload{CntOptionContents: []string{"yes"}},
		}},
		InterpretRules: []sharedpayload.InterpretRule{{
			DimensionCode: "total",
			Ranges:        []sharedpayload.ScoreRangeRule{{MinScore: 0, MaxScore: 10, Level: "low"}},
		}},
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	got, err := sharedpayload.ParseDefinitionBodyJSON(raw)
	if err != nil {
		t.Fatalf("ParseDefinitionBodyJSON: %v", err)
	}
	if len(got.Dimensions) != 1 || got.Dimensions[0].Code != "total" {
		t.Fatalf("dimensions = %#v", got.Dimensions)
	}
	if got.Dimensions[0].ScoringParams == nil || len(got.Dimensions[0].ScoringParams.CntOptionContents) != 1 {
		t.Fatalf("scoring params = %#v", got.Dimensions[0].ScoringParams)
	}
}

func TestMeasureSpecFromDefinitionBodyClonesMutablePayloadState(t *testing.T) {
	t.Parallel()

	maxScore := 10.0
	weights := map[string]float64{"f1": 0.4}
	body := sharedpayload.DefinitionBody{Dimensions: []sharedpayload.DimensionRule{{
		Code: "total", Title: "总分", IsTotalScore: true,
		ScoringParams: &sharedpayload.ScoringParamsPayload{CntOptionContents: []string{"yes"}},
		MaxScore:      &maxScore,
		ChildrenPolicy: &sharedpayload.ChildrenPolicyPayload{
			Strategy: string(factor.ChildrenAggregationWeightedSum),
			Children: []string{"f1"},
			Weights:  weights,
		},
	}}}
	measure := sharedpayload.MeasureSpecFromDefinitionBody(body)
	maxScore = 99
	weights["f1"] = 9.9
	if len(measure.Factors) != 1 || measure.Factors[0].ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("factors = %#v", measure.Factors)
	}
	if len(measure.Scoring) != 1 || *measure.Scoring[0].MaxScore != 10 || measure.Scoring[0].Weights["f1"] != 0.4 {
		t.Fatalf("scoring shares payload state: %#v", measure.Scoring)
	}
	if len(measure.FactorGraph.Edges) != 1 || measure.FactorGraph.Edges[0].ChildCode != "f1" {
		t.Fatalf("graph = %#v", measure.FactorGraph)
	}
}

func TestScoreRangeRuleMatchesLeftClosedRightOpen(t *testing.T) {
	t.Parallel()

	rule := sharedpayload.ScoreRangeRule{MinScore: 0, MaxScore: 10}
	if !rule.Matches(0) || !rule.Matches(9.9) || rule.Matches(10) {
		t.Fatal("expected [0,10) semantics")
	}
}

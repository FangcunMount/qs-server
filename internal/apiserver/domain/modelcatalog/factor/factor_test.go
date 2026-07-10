package factor_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestFactorKeepsSlimIdentityShape(t *testing.T) {
	t.Parallel()

	if got := reflect.TypeOf(factor.Factor{}).NumField(); got != 3 {
		t.Fatalf("Factor field count = %d, want 3 identity fields", got)
	}
}

func TestParseFactorsFromDefinitionBody(t *testing.T) {
	t.Parallel()

	maxScore := 10.0
	weights := map[string]float64{"f1": 0.4}
	questionCodes := []string{"q1", "q2"}
	ranges := []factor.ScoreRangeRule{{
		MinScore: 0, MaxScore: 10, Conclusion: "low", Level: "low",
	}}
	dimensions := []factor.DimensionRule{{
		Code:            "total",
		Title:           "总分",
		QuestionCodes:   questionCodes,
		ScoringStrategy: "sum",
		ScoringParams:   &factor.ScoringParamsPayload{CntOptionContents: []string{"yes"}},
		MaxScore:        &maxScore,
		IsTotalScore:    true,
		ChildrenPolicy: &factor.ChildrenPolicyPayload{
			Strategy: string(factor.ChildrenAggregationWeightedSum),
			Children: []string{"f1"},
			Weights:  weights,
		},
	}}
	interpretRules := []factor.InterpretRule{{
		DimensionCode: "total",
		Ranges:        ranges,
	}}
	factors := factor.ParseFactorsFromDefinitionBody(dimensions, interpretRules)
	scoring := factor.ScoringFromDefinitionDimensions(dimensions)
	graph := factor.FactorGraphFromDefinitionDimensions(dimensions)
	if len(factors) != 1 {
		t.Fatalf("factors = %#v", factors)
	}
	if factors[0].Code != "total" || factors[0].Title != "总分" {
		t.Fatalf("factor = %#v", factors[0])
	}
	if factors[0].ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("role = %s", factors[0].ResolvedRole())
	}
	if len(scoring) != 1 || scoring[0].Strategy != factor.ScoringStrategyWeightedSum {
		t.Fatalf("scoring = %#v", scoring)
	}
	questionCodes[0] = "mutated"
	ranges[0].Level = "mutated"
	maxScore = 99
	weights["f1"] = 9.9
	if scoring[0].Sources[0].Code != "f1" ||
		*scoring[0].MaxScore != 10 ||
		scoring[0].Weights["f1"] != 0.4 ||
		graph.Edges[0].ChildCode != "f1" {
		t.Fatalf("definition body parse shares mutable state: scoring=%#v graph=%#v", scoring[0], graph)
	}
}

func TestScoreRangeRuleMatchesLeftClosedRightOpen(t *testing.T) {
	t.Parallel()

	rule := factor.ScoreRangeRule{MinScore: 0, MaxScore: 10}
	if !rule.Matches(0) || !rule.Matches(9.9) || rule.Matches(10) {
		t.Fatal("expected [0,10) semantics")
	}
}

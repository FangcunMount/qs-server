package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

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
	legacy := factor.ParseLegacyFactorsFromDefinitionBody(dimensions, interpretRules)
	if len(factors) != 1 {
		t.Fatalf("factors = %#v", factors)
	}
	if factors[0].Code != "total" || factors[0].Title != "总分" {
		t.Fatalf("factor = %#v", factors[0])
	}
	if factors[0].ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("role = %s", factors[0].ResolvedRole())
	}
	if len(legacy) != 1 || legacy[0].ScoringStrategy != "sum" {
		t.Fatalf("legacy = %#v", legacy)
	}
	if len(legacy[0].InterpretRules) != 1 || legacy[0].InterpretRules[0].Level != "low" {
		t.Fatalf("rules = %#v", legacy[0].InterpretRules)
	}
	snapshots := factor.ParseFactorSnapshotsFromDefinitionBody(dimensions, interpretRules)
	questionCodes[0] = "mutated"
	ranges[0].Level = "mutated"
	maxScore = 99
	weights["f1"] = 9.9
	if legacy[0].QuestionCodes[0] != "q1" ||
		legacy[0].InterpretRules[0].Level != "low" ||
		*legacy[0].MaxScore != 10 ||
		legacy[0].ChildrenPolicy.Weights["f1"] != 0.4 {
		t.Fatalf("definition body parse shares mutable state: %#v", legacy[0])
	}
	if len(snapshots) != 1 || snapshots[0].QuestionCodes[0] != "q1" {
		t.Fatalf("snapshots = %#v", snapshots)
	}
}

func TestScoreRangeRuleMatchesLeftClosedRightOpen(t *testing.T) {
	t.Parallel()

	rule := factor.ScoreRangeRule{MinScore: 0, MaxScore: 10}
	if !rule.Matches(0) || !rule.Matches(9.9) || rule.Matches(10) {
		t.Fatal("expected [0,10) semantics")
	}
}

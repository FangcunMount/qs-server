package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestParseFactorsFromDefinitionBody(t *testing.T) {
	t.Parallel()

	factors := factor.ParseFactorsFromDefinitionBody(
		[]factor.DimensionRule{{
			Code:            "total",
			Title:           "总分",
			QuestionCodes:   []string{"q1", "q2"},
			ScoringStrategy: "sum",
			IsTotalScore:    true,
		}},
		[]factor.InterpretRule{{
			DimensionCode: "total",
			Ranges: []factor.ScoreRangeRule{{
				MinScore: 0, MaxScore: 10, Conclusion: "low", Level: "low",
			}},
		}},
	)
	if len(factors) != 1 {
		t.Fatalf("factors = %#v", factors)
	}
	if factors[0].Code != "total" || factors[0].ScoringStrategy != "sum" {
		t.Fatalf("factor = %#v", factors[0])
	}
	if factors[0].ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("role = %s", factors[0].ResolvedRole())
	}
	if len(factors[0].InterpretRules) != 1 || factors[0].InterpretRules[0].Level != "low" {
		t.Fatalf("rules = %#v", factors[0].InterpretRules)
	}
}

func TestScoreRangeRuleMatchesLeftClosedRightOpen(t *testing.T) {
	t.Parallel()

	rule := factor.ScoreRangeRule{MinScore: 0, MaxScore: 10}
	if !rule.Matches(0) || !rule.Matches(9.9) || rule.Matches(10) {
		t.Fatal("expected [0,10) semantics")
	}
}

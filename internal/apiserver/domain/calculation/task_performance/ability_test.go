package task_performance

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
)

func TestApplyAbilityConclusionsProjectsMatchingRawScoreRange(t *testing.T) {
	t.Parallel()
	got := ApplyAbilityConclusions(calculation.Result{
		Dimensions: []calculation.DimensionResult{{
			Code: "total", Role: "total",
			Score: &calculation.ScoreValue{Kind: calculation.ScoreKindRawTotal, Value: 42},
		}},
	}, []AbilityRule{{
		FactorCode: "total", ScoreBasis: ScoreBasisRaw, Primary: true,
		Ranges: []AbilityRange{{
			Bound: scorerange.Bound{Min: 40, Max: 50, MaxInclusive: true},
			Level: "high", OutcomeCode: "ability_high",
		}},
	}})
	if got.Dimensions[0].Level == nil || got.Dimensions[0].Level.Code != "ability_high" {
		t.Fatalf("level = %#v", got.Dimensions[0].Level)
	}
	if got.Level == nil || got.Level.Code != "ability_high" {
		t.Fatalf("result level = %#v", got.Level)
	}
}

func TestApplyAbilityConclusionsLeavesUnconfiguredScoreBasisUntouched(t *testing.T) {
	t.Parallel()
	got := ApplyAbilityConclusions(calculation.Result{
		Dimensions: []calculation.DimensionResult{{
			Code:  "total",
			Score: &calculation.ScoreValue{Kind: calculation.ScoreKindRawTotal, Value: 42},
		}},
	}, []AbilityRule{{
		FactorCode: "total", ScoreBasis: ScoreBasisTScore,
		Ranges: []AbilityRange{{Bound: scorerange.Bound{Min: 40, Max: 50, MaxInclusive: true}, Level: "high"}},
	}})
	if got.Dimensions[0].Level != nil {
		t.Fatalf("level = %#v, want nil", got.Dimensions[0].Level)
	}
}

func TestApplyAbilityConclusionsBoundaryGolden(t *testing.T) {
	t.Parallel()
	rules := []AbilityRule{{
		FactorCode: "total", ScoreBasis: ScoreBasisRaw, Primary: true,
		Ranges: []AbilityRange{
			{Bound: scorerange.Bound{Min: 0, Max: 40}, OutcomeCode: "low"},
			{Bound: scorerange.Bound{Min: 40, Max: 100, MaxInclusive: true}, OutcomeCode: "high"},
		},
	}}
	cases := []struct {
		name  string
		score float64
		code  string
		ok    bool
	}{
		{name: "below minimum", score: -1, ok: false},
		{name: "minimum inclusive", score: 0, code: "low", ok: true},
		{name: "just below boundary", score: 39.9, code: "low", ok: true},
		{name: "boundary belongs to next", score: 40, code: "high", ok: true},
		{name: "maximum inclusive", score: 100, code: "high", ok: true},
		{name: "above maximum", score: 100.1, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyAbilityConclusions(calculation.Result{
				Dimensions: []calculation.DimensionResult{{
					Code: "total", Role: "total",
					Score: &calculation.ScoreValue{Value: tc.score},
				}},
			}, rules)
			if !tc.ok {
				if got.Dimensions[0].Level != nil || got.Level != nil {
					t.Fatalf("level = dim %#v result %#v, want nil", got.Dimensions[0].Level, got.Level)
				}
				return
			}
			if got.Dimensions[0].Level == nil || got.Dimensions[0].Level.Code != tc.code {
				t.Fatalf("level = %#v, want %s", got.Dimensions[0].Level, tc.code)
			}
		})
	}
}

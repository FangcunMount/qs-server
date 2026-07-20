package task_performance

import (
	"testing"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
)

func TestApplyAbilityConclusionsProjectsMatchingRawScoreRange(t *testing.T) {
	outcome := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
		Code: "total", Role: "total", Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 42},
	}}}
	got := ApplyAbilityConclusions(outcome, []conclusion.AbilityConclusion{{
		FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw, Primary: true,
		Rules: []conclusion.ScoreRangeOutcome{{MinScore: 40, MaxScore: 50, Level: "high", OutcomeCode: "ability_high", Title: "优秀", Summary: "能力较强", Description: "继续保持", MaxInclusive: true}},
	}})
	if got.Dimensions[0].Level == nil || got.Dimensions[0].Level.Code != "ability_high" || got.Dimensions[0].Level.Label != "" {
		t.Fatalf("level = %#v", got.Dimensions[0].Level)
	}
	if got.Level == nil || got.Level.Code != "ability_high" {
		t.Fatalf("execution level = %#v", got.Level)
	}
	decision := domainoutcome.DecisionResultFromExecution(got)
	if decision.OutcomeCode != "ability_high" || decision.LevelCode != "ability_high" {
		t.Fatalf("decision = %#v", decision)
	}
	if decision.LevelLabel != "" {
		t.Fatalf("LevelLabel must not carry presentation copy, got %q", decision.LevelLabel)
	}
}

func TestApplyAbilityConclusionsLeavesUnconfiguredScoreBasisUntouched(t *testing.T) {
	outcome := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
		Code: "total", Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 42},
	}}}
	got := ApplyAbilityConclusions(outcome, []conclusion.AbilityConclusion{{
		FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore,
		Rules: []conclusion.ScoreRangeOutcome{{MinScore: 40, MaxScore: 50, Level: "high", MaxInclusive: true}},
	}})
	if got.Dimensions[0].Level != nil {
		t.Fatalf("level = %#v, want nil", got.Dimensions[0].Level)
	}
}

func TestApplyAbilityConclusionsBoundaryGolden(t *testing.T) {
	t.Parallel()

	rules := []conclusion.AbilityConclusion{{
		FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw, Primary: true,
		Rules: []conclusion.ScoreRangeOutcome{
			{MinScore: 0, MaxScore: 40, OutcomeCode: "low", Title: "偏低"},
			{MinScore: 40, MaxScore: 100, OutcomeCode: "high", Title: "偏高", MaxInclusive: true},
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
			outcome := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
				Code: "total", Role: "total", Score: &domainoutcome.ScoreValue{Value: tc.score},
			}}}
			got := ApplyAbilityConclusions(outcome, rules)
			if !tc.ok {
				if got.Dimensions[0].Level != nil || got.Level != nil {
					t.Fatalf("level = dim %#v exec %#v, want nil", got.Dimensions[0].Level, got.Level)
				}
				return
			}
			if got.Dimensions[0].Level == nil || got.Dimensions[0].Level.Code != tc.code {
				t.Fatalf("dim level = %#v, want %s", got.Dimensions[0].Level, tc.code)
			}
			if got.Level == nil || got.Level.Code != tc.code {
				t.Fatalf("exec level = %#v, want %s", got.Level, tc.code)
			}
		})
	}
}

func TestApplyAbilityConclusionsUnboundedMax(t *testing.T) {
	t.Parallel()

	got := ApplyAbilityConclusions(&domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
		Code: "total", Role: "total", Score: &domainoutcome.ScoreValue{Value: 999},
	}}}, []conclusion.AbilityConclusion{{
		FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw, Primary: true,
		Rules: []conclusion.ScoreRangeOutcome{
			{MinScore: 0, MaxScore: 90, OutcomeCode: "mid"},
			{MinScore: 90, OutcomeCode: "top", Title: "顶尖", UnboundedMax: true},
		},
	}})
	if got.Level == nil || got.Level.Code != "top" {
		t.Fatalf("level = %#v", got.Level)
	}
}

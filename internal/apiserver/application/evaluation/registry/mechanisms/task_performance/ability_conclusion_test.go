package task_performance

import (
	"testing"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
)

func TestApplyAbilityConclusionsProjectsMatchingRawScoreRange(t *testing.T) {
	outcome := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
		Code: "total", Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 42},
	}}}
	got := ApplyAbilityConclusions(outcome, []conclusion.AbilityConclusion{{
		FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw,
		Rules: []conclusion.ScoreRangeOutcome{{MinScore: 40, MaxScore: 50, Level: "high", Title: "优秀", Summary: "能力较强", Description: "继续保持"}},
	}})
	if got.Dimensions[0].Level == nil || got.Dimensions[0].Level.Code != "high" {
		t.Fatalf("level = %#v", got.Dimensions[0].Level)
	}
}

func TestApplyAbilityConclusionsLeavesUnconfiguredScoreBasisUntouched(t *testing.T) {
	outcome := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
		Code: "total", Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 42},
	}}}
	got := ApplyAbilityConclusions(outcome, []conclusion.AbilityConclusion{{
		FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore,
		Rules: []conclusion.ScoreRangeOutcome{{MinScore: 40, MaxScore: 50, Level: "high"}},
	}})
	if got.Dimensions[0].Level != nil {
		t.Fatalf("level = %#v, want nil", got.Dimensions[0].Level)
	}
}

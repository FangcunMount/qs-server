package task_performance

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
)

func TestApplyAbilityConclusionsProjectsMatchingRawScoreRange(t *testing.T) {
	outcome := &assessment.AssessmentOutcome{Dimensions: []assessment.DimensionResult{{
		Code: "total", Score: &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 42},
	}}}
	got := ApplyAbilityConclusions(outcome, []conclusion.AbilityConclusion{{
		FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw,
		Rules: []conclusion.ScoreRangeOutcome{{MinScore: 40, MaxScore: 50, Level: "high", Title: "优秀", Summary: "能力较强", Description: "继续保持"}},
	}})
	if got.Dimensions[0].Level == nil || got.Dimensions[0].Level.Code != "high" {
		t.Fatalf("level = %#v", got.Dimensions[0].Level)
	}
	if got.Dimensions[0].Description != "能力较强" || got.Dimensions[0].Suggestion != "继续保持" {
		t.Fatalf("dimension = %#v", got.Dimensions[0])
	}
}

func TestApplyAbilityConclusionsLeavesUnconfiguredScoreBasisUntouched(t *testing.T) {
	outcome := &assessment.AssessmentOutcome{Dimensions: []assessment.DimensionResult{{
		Code: "total", Score: &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 42},
	}}}
	got := ApplyAbilityConclusions(outcome, []conclusion.AbilityConclusion{{
		FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore,
		Rules: []conclusion.ScoreRangeOutcome{{MinScore: 40, MaxScore: 50, Level: "high"}},
	}})
	if got.Dimensions[0].Level != nil {
		t.Fatalf("level = %#v, want nil", got.Dimensions[0].Level)
	}
}

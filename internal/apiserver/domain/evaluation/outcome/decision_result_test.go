package outcome_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func TestDecisionResultFromExecution(t *testing.T) {
	t.Parallel()

	levelCode := "ability_high"
	execution := &outcome.Execution{
		Summary: outcome.Summary{PrimaryLabel: "优秀", Level: &levelCode},
		Primary: &outcome.ScoreValue{Kind: outcome.ScoreKindRawTotal, Value: 42},
		Level:   &outcome.ResultLevel{Code: "ability_high", Label: "优秀"},
		Dimensions: []outcome.DimensionResult{{
			Code: "total", Level: &outcome.ResultLevel{Code: "ability_high", Label: "优秀"},
		}},
	}
	got := outcome.DecisionResultFromExecution(execution)
	if got.OutcomeCode != "ability_high" || got.LevelCode != "ability_high" || got.LevelLabel != "优秀" {
		t.Fatalf("decision = %#v", got)
	}
	if got.PrimaryScore == nil || got.PrimaryScore.Value != 42 {
		t.Fatalf("primary = %#v", got.PrimaryScore)
	}
	if len(got.Dimensions) != 1 {
		t.Fatalf("dimensions = %d", len(got.Dimensions))
	}
}

func TestDecisionResultFromExecutionUsesProfileCode(t *testing.T) {
	t.Parallel()

	got := outcome.DecisionResultFromExecution(&outcome.Execution{
		Profile: &outcome.ProfileResult{Kind: outcome.ProfileKindPersonalityType, Code: "INTJ", Name: "建筑师"},
	})
	if got.OutcomeCode != "INTJ" {
		t.Fatalf("outcome code = %q", got.OutcomeCode)
	}
}

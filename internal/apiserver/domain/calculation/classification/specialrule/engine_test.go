package specialrule_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification/specialrule"
)

func TestEngineApplyBeforeScoreMatchesDrinkTrigger(t *testing.T) {
	rules := []specialrule.Rule{
		{
			Code: "DRUNK",
			Kind: specialrule.RuleKindAnswerMatch,
			Condition: specialrule.Condition{
				QuestionCodes: []string{"drink_gate_q2"},
				OptionValues:  []string{"C"},
			},
		},
	}
	outcomes := []specialrule.Outcome{
		{Code: "DRUNK", Trigger: "hidden:drink"},
	}

	got, ok := (specialrule.Engine{}).ApplyBeforeScore(rules, outcomes, []classification.Answer{
		{QuestionCode: "drink_gate_q2", Value: "C"},
	})
	if !ok {
		t.Fatal("expected drink rule match")
	}
	if got.OutcomeCode != "DRUNK" || !got.SkipScoring {
		t.Fatalf("match = %#v", got)
	}
}

func TestEngineApplyAfterDecisionUsesFallbackOutcome(t *testing.T) {
	rules := []specialrule.Rule{
		{Kind: specialrule.RuleKindFallbackThreshold, Phase: specialrule.RuleAfterDecision},
	}
	decision := specialrule.Decision{
		FallbackSimilarityThreshold: 0.9,
		FallbackCode:                "HHHH",
	}
	outcomes := []specialrule.Outcome{
		{Code: "HHHH", Trigger: "fallback:best_match<60%"},
	}

	got, ok := (specialrule.Engine{}).ApplyAfterDecision(rules, decision, outcomes, 0.5)
	if !ok {
		t.Fatal("expected fallback match")
	}
	if got.OutcomeCode != "HHHH" || got.Trigger != "fallback:best_match<60%" {
		t.Fatalf("match = %#v", got)
	}
}

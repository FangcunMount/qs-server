package typology

import "testing"

func TestSpecialRuleSpecResolvedKind(t *testing.T) {
	t.Run("explicit answer_match", func(t *testing.T) {
		rule := SpecialRuleSpec{
			Kind: SpecialRuleKindAnswerMatch,
			Condition: SpecialRuleCondition{
				QuestionCodes: []string{"q1"},
				OptionValues:  []string{"A"},
			},
		}
		if rule.ResolvedKind() != SpecialRuleKindAnswerMatch {
			t.Fatalf("ResolvedKind() = %s", rule.ResolvedKind())
		}
		if len(rule.ResolvedQuestionCodes()) != 1 || rule.ResolvedQuestionCodes()[0] != "q1" {
			t.Fatalf("ResolvedQuestionCodes() = %#v", rule.ResolvedQuestionCodes())
		}
	})

	t.Run("legacy flat fields infer answer_match", func(t *testing.T) {
		rule := SpecialRuleSpec{
			Phase:         SpecialRuleBeforeScore,
			QuestionCodes: []string{"q1"},
			OptionValues:  []string{"B"},
		}
		if rule.ResolvedKind() != SpecialRuleKindAnswerMatch {
			t.Fatalf("ResolvedKind() = %s", rule.ResolvedKind())
		}
	})

	t.Run("after_decision infers fallback_threshold", func(t *testing.T) {
		rule := SpecialRuleSpec{
			Phase:       SpecialRuleAfterDecision,
			OutcomeCode: "LOW_MATCH",
		}
		if rule.ResolvedKind() != SpecialRuleKindFallbackThreshold {
			t.Fatalf("ResolvedKind() = %s", rule.ResolvedKind())
		}
	})
}

func TestFallbackCodeFromOutcomesNoDefault(t *testing.T) {
	if got := fallbackCodeFromOutcomes(nil); got != "" {
		t.Fatalf("fallbackCodeFromOutcomes(nil) = %q, want empty", got)
	}
	if got := fallbackCodeFromOutcomes([]Outcome{
		{Code: "SPECIAL", IsSpecial: true, Trigger: "hidden:drink"},
	}); got != "" {
		t.Fatalf("fallbackCodeFromOutcomes(hidden) = %q, want empty", got)
	}
	if got := fallbackCodeFromOutcomes([]Outcome{
		{Code: "HHHH", IsSpecial: true, Trigger: "fallback:best_match<60%"},
	}); got != "HHHH" {
		t.Fatalf("fallbackCodeFromOutcomes(fallback) = %q, want HHHH", got)
	}
}

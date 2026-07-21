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

	t.Run("kind must be explicit", func(t *testing.T) {
		rule := SpecialRuleSpec{
			Phase: SpecialRuleBeforeScore,
			Condition: SpecialRuleCondition{
				QuestionCodes: []string{"q1"},
				OptionValues:  []string{"B"},
			},
		}
		if rule.ResolvedKind() != "" {
			t.Fatalf("ResolvedKind() = %s, want empty", rule.ResolvedKind())
		}
	})

	t.Run("explicit fallback_threshold", func(t *testing.T) {
		rule := SpecialRuleSpec{
			Phase:       SpecialRuleAfterDecision,
			Kind:        SpecialRuleKindFallbackThreshold,
			OutcomeCode: "LOW_MATCH",
		}
		if rule.ResolvedKind() != SpecialRuleKindFallbackThreshold {
			t.Fatalf("ResolvedKind() = %s", rule.ResolvedKind())
		}
	})
}

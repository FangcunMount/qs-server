package interpretation_test

import (
	"testing"

	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestMatchRuleUsesInclusiveUpperBoundLikeScaleRiskRules(t *testing.T) {
	rules := []domaininterpretation.ScoreRangeRule{
		{Min: 0, Max: 10, Level: "low", Conclusion: "low", Suggestion: "keep"},
		{Min: 10, Max: 20, Level: "high", Conclusion: "high", Suggestion: "act"},
	}
	if got := domaininterpretation.MatchRule(10, rules); got == nil || got.Level != "low" {
		t.Fatalf("score 10 = %#v, want low bucket", got)
	}
	if got := domaininterpretation.MatchRule(20, rules); got == nil || got.Level != "high" {
		t.Fatalf("score 20 = %#v, want high bucket", got)
	}
}

func TestInterpretRuleUsesHalfOpenInterval(t *testing.T) {
	rule := domaininterpretation.InterpretRule{Min: 0, Max: 10, RiskLevel: domaininterpretation.RiskLevelLow}
	if !rule.Contains(0) || rule.Contains(10) || !rule.Contains(9.9) {
		t.Fatalf("half-open [0,10) mismatch for %#v", rule)
	}
}

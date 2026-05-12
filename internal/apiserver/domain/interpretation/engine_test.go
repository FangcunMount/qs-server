package interpretation

import "testing"

func TestMatchRuleWithRangeFallback(t *testing.T) {
	rules := []ScoreRangeRule{
		{Min: 0, Max: 10, Level: "low", Conclusion: "low"},
		{Min: 11, Max: 20, Level: "high", Conclusion: "high"},
	}
	if got := MatchRule(5, rules); got == nil || got.Level != "low" {
		t.Fatalf("MatchRule low failed, got %#v", got)
	}
	if got := MatchRuleWithRangeFallback(100, rules); got == nil || got.Level != "high" {
		t.Fatalf("MatchRuleWithRangeFallback failed, got %#v", got)
	}
}

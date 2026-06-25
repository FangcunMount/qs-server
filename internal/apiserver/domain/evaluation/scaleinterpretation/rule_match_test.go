package scaleinterpretation

import "testing"

func TestMatchScoreRuleWithRangeFallback(t *testing.T) {
	rules := []scoreRangeRule{
		{Min: 0, Max: 10, Level: "low", Conclusion: "low"},
		{Min: 11, Max: 20, Level: "high", Conclusion: "high"},
	}
	if got := matchScoreRule(5, rules); got == nil || got.Level != "low" {
		t.Fatalf("matchScoreRule low failed, got %#v", got)
	}
	if got := matchScoreRuleWithRangeFallback(100, rules); got == nil || got.Level != "high" {
		t.Fatalf("matchScoreRuleWithRangeFallback failed, got %#v", got)
	}
}

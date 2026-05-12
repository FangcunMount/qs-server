package interpretation

type ScoreRangeRule struct {
	Min        float64
	Max        float64
	Level      string
	Conclusion string
	Suggestion string
}

func MatchRule(score float64, rules []ScoreRangeRule) *ScoreRangeRule {
	for i := range rules {
		if score >= rules[i].Min && score <= rules[i].Max {
			return &rules[i]
		}
	}
	return nil
}

func MatchRuleWithRangeFallback(score float64, rules []ScoreRangeRule) *ScoreRangeRule {
	if rule := MatchRule(score, rules); rule != nil {
		return rule
	}
	if len(rules) == 0 {
		return nil
	}
	return &rules[len(rules)-1]
}

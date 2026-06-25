package scaleinterpretation

type scoreRangeRule struct {
	Min        float64
	Max        float64
	Level      string
	Conclusion string
	Suggestion string
}

func matchScoreRule(score float64, rules []scoreRangeRule) *scoreRangeRule {
	for i := range rules {
		if score >= rules[i].Min && score <= rules[i].Max {
			return &rules[i]
		}
	}
	return nil
}

func matchScoreRuleWithRangeFallback(score float64, rules []scoreRangeRule) *scoreRangeRule {
	if rule := matchScoreRule(score, rules); rule != nil {
		return rule
	}
	if len(rules) == 0 {
		return nil
	}
	return &rules[len(rules)-1]
}

package scaleinterpretation

import rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"

func findInterpretRule(factor rulesetscale.FactorSnapshot, score float64) *rulesetscale.InterpretRuleSnapshot {
	for i := range factor.InterpretRules {
		if factor.InterpretRules[i].Matches(score) {
			return &factor.InterpretRules[i]
		}
	}
	return nil
}

func findInterpretRuleWithRangeFallback(factor rulesetscale.FactorSnapshot, score float64) *rulesetscale.InterpretRuleSnapshot {
	if rule := findInterpretRule(factor, score); rule != nil {
		return rule
	}
	if len(factor.InterpretRules) == 0 {
		return nil
	}
	last := factor.InterpretRules[len(factor.InterpretRules)-1]
	return &last
}

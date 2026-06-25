package scale

import scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"

func findInterpretRule(factor scalesnapshot.FactorSnapshot, score float64) *scalesnapshot.InterpretRuleSnapshot {
	for i := range factor.InterpretRules {
		if factor.InterpretRules[i].Matches(score) {
			return &factor.InterpretRules[i]
		}
	}
	return nil
}

func findInterpretRuleWithRangeFallback(factor scalesnapshot.FactorSnapshot, score float64) *scalesnapshot.InterpretRuleSnapshot {
	if rule := findInterpretRule(factor, score); rule != nil {
		return rule
	}
	if len(factor.InterpretRules) == 0 {
		return nil
	}
	last := factor.InterpretRules[len(factor.InterpretRules)-1]
	return &last
}

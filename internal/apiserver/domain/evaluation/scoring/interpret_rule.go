package scoring

import scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"

func findInterpretRule(factor scalesnapshot.FactorSnapshot, score float64) *scalesnapshot.InterpretRuleSnapshot {
	for i := range factor.InterpretRules {
		if factor.InterpretRules[i].Matches(score) {
			return &factor.InterpretRules[i]
		}
	}
	return nil
}

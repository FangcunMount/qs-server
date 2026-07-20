package scoring

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"

func findInterpretRule(factor Factor, score float64) *InterpretRule {
	if len(factor.InterpretRules) == 0 {
		return nil
	}
	bounds := make([]scorerange.Bound, len(factor.InterpretRules))
	for i, rule := range factor.InterpretRules {
		bounds[i] = scorerange.Bound{
			Min: rule.Min, Max: rule.Max, MaxInclusive: rule.MaxInclusive, UnboundedMax: rule.UnboundedMax,
		}
	}
	index, ok := scorerange.MatchBounds(score, bounds)
	if !ok {
		return nil
	}
	return &factor.InterpretRules[index]
}

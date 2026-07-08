package scoring

func findInterpretRule(factor Factor, score float64) *InterpretRule {
	for i := range factor.InterpretRules {
		if factor.InterpretRules[i].Matches(score) {
			return &factor.InterpretRules[i]
		}
	}
	return nil
}

package factor

func cloneStrings(items []string) []string {
	if items == nil {
		return nil
	}
	return append([]string(nil), items...)
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneScoringParams(params *ScoringParams) *ScoringParams {
	if params == nil {
		return nil
	}
	return &ScoringParams{
		CntOptionContents: cloneStrings(params.CntOptionContents),
	}
}

func cloneChildrenPolicy(policy *ChildrenPolicy) *ChildrenPolicy {
	if policy == nil {
		return nil
	}
	weights := map[string]float64(nil)
	if policy.Weights != nil {
		weights = make(map[string]float64, len(policy.Weights))
		for key, value := range policy.Weights {
			weights[key] = value
		}
	}
	return &ChildrenPolicy{
		Strategy: policy.Strategy,
		Children: cloneStrings(policy.Children),
		Weights:  weights,
	}
}

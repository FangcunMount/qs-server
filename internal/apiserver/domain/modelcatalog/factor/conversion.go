package factor

// FactorFromSnapshot materializes the domain Factor from a compatibility snapshot.
func FactorFromSnapshot(snapshot FactorSnapshot) Factor {
	return Factor{
		Code:            snapshot.Code,
		Title:           snapshot.Title,
		Role:            snapshot.Role,
		ParentCode:      snapshot.ParentCode,
		SortOrder:       snapshot.SortOrder,
		Level:           snapshot.Level,
		IsTotalScore:    snapshot.IsTotalScore,
		QuestionCodes:   cloneStrings(snapshot.QuestionCodes),
		ScoringStrategy: snapshot.ScoringStrategy,
		ScoringParams:   cloneScoringParams(snapshot.ScoringParams),
		MaxScore:        cloneFloat64(snapshot.MaxScore),
		InterpretRules:  cloneScoreRangeRules(snapshot.InterpretRules),
		Classification:  cloneClassificationSpec(snapshot.Classification),
		Norm:            cloneNormRef(snapshot.Norm),
		ChildrenPolicy:  cloneChildrenPolicy(snapshot.ChildrenPolicy),
	}
}

// FactorsFromSnapshots materializes domain Factors from compatibility snapshots.
func FactorsFromSnapshots(snapshots []FactorSnapshot) []Factor {
	if snapshots == nil {
		return nil
	}
	out := make([]Factor, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, FactorFromSnapshot(snapshot))
	}
	return out
}

// SnapshotsFromFactors returns compatibility snapshots for domain Factors.
func SnapshotsFromFactors(factors []Factor) []FactorSnapshot {
	if factors == nil {
		return nil
	}
	out := make([]FactorSnapshot, 0, len(factors))
	for _, item := range factors {
		out = append(out, item.Snapshot())
	}
	return out
}

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

func cloneScoreRangeRules(rules []ScoreRangeRule) []ScoreRangeRule {
	if rules == nil {
		return nil
	}
	return append([]ScoreRangeRule(nil), rules...)
}

func cloneClassificationSpec(spec *ClassificationSpec) *ClassificationSpec {
	if spec == nil {
		return nil
	}
	cloned := *spec
	return &cloned
}

func cloneNormRef(ref *NormRef) *NormRef {
	if ref == nil {
		return nil
	}
	cloned := *ref
	return &cloned
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

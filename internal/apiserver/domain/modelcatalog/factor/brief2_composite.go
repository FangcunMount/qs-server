package factor

// Brief2CompositeIndexSpec declares how a Brief-2 composite index derives from child factors.
type Brief2CompositeIndexSpec struct {
	Code       string
	Strategy   ChildrenAggregationStrategy
	Children   []string
	ParentCode string
}

// ApplyBrief2CompositeMetadata annotates factors with Brief-2 composite index policies.
func ApplyBrief2CompositeMetadata(factors []FactorSnapshot, specs []Brief2CompositeIndexSpec) []FactorSnapshot {
	if len(factors) == 0 || len(specs) == 0 {
		return factors
	}
	out := make([]FactorSnapshot, len(factors))
	copy(out, factors)
	indexPos := make(map[string]int, len(out))
	for i, item := range out {
		indexPos[item.Code] = i
	}
	for _, spec := range specs {
		pos, ok := indexPos[spec.Code]
		if !ok || len(spec.Children) == 0 {
			continue
		}
		strategy := spec.Strategy
		if strategy == "" {
			strategy = ChildrenAggregationSum
		}
		out[pos].ChildrenPolicy = &ChildrenPolicy{
			Strategy: strategy,
			Children: append([]string(nil), spec.Children...),
		}
		if spec.ParentCode != "" {
			out[pos].ParentCode = spec.ParentCode
		}
		for _, childCode := range spec.Children {
			childPos, ok := indexPos[childCode]
			if !ok || out[childPos].ParentCode != "" {
				continue
			}
			out[childPos].ParentCode = spec.Code
		}
	}
	return DeriveLevels(out)
}

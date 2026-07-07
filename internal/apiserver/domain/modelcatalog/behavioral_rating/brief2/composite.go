package brief2

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// CompositeIndexSpec declares how a Brief-2 composite index derives from child factors.
type CompositeIndexSpec struct {
	Code       string
	Strategy   factor.ChildrenAggregationStrategy
	Children   []string
	ParentCode string
}

// ApplyCompositeMetadata annotates factors with Brief-2 composite index policies.
func ApplyCompositeMetadata(factors []factor.FactorSnapshot, specs []CompositeIndexSpec) []factor.FactorSnapshot {
	if len(factors) == 0 || len(specs) == 0 {
		return factors
	}
	out := make([]factor.FactorSnapshot, len(factors))
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
			strategy = factor.ChildrenAggregationSum
		}
		out[pos].ChildrenPolicy = &factor.ChildrenPolicy{
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
	return factor.DeriveLevels(out)
}

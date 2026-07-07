package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// ScoreNodesFromFactors translates 目录 因子快照 为 计算 ScoreNodes。
func ScoreNodesFromFactors(factors []factor.FactorSnapshot) []calculation.ScoreNode {
	if len(factors) == 0 {
		return nil
	}
	inferred := factor.InferParentCodesFromChildrenPolicy(factors)
	nodes := make([]calculation.ScoreNode, 0, len(inferred))
	for _, f := range inferred {
		role := f.ResolvedRole()
		node := calculation.ScoreNode{
			Code:       f.Code,
			Name:       f.Title,
			Role:       string(role),
			Kind:       dimensionKindForRole(role),
			ParentCode: f.ParentCode,
			Level:      f.Level,
			SortOrder:  f.SortOrder,
		}
		if f.ChildrenPolicy != nil {
			node.Aggregation = aggregationFromChildrenStrategy(f.ChildrenPolicy.Strategy)
			node.Children = append([]string(nil), f.ChildrenPolicy.Children...)
			if len(f.ChildrenPolicy.Weights) > 0 {
				node.Weights = make(map[string]float64, len(f.ChildrenPolicy.Weights))
				for code, weight := range f.ChildrenPolicy.Weights {
					node.Weights[code] = weight
				}
			}
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func dimensionKindForRole(role factor.FactorRole) calculation.DimensionKind {
	if role.Resolved() == factor.FactorRoleIndex {
		return calculation.DimensionKindIndex
	}
	return calculation.DimensionKindFactor
}

func aggregationFromChildrenStrategy(strategy factor.ChildrenAggregationStrategy) calculation.AggregationStrategy {
	switch strategy {
	case factor.ChildrenAggregationSum:
		return calculation.AggregationSum
	case factor.ChildrenAggregationAverage:
		return calculation.AggregationAverage
	case factor.ChildrenAggregationWeightedSum:
		return calculation.AggregationWeightedSum
	case factor.ChildrenAggregationLookup:
		return calculation.AggregationLookup
	case factor.ChildrenAggregationCustom:
		return calculation.AggregationCustom
	case factor.ChildrenAggregationNone:
		return calculation.AggregationNone
	default:
		return ""
	}
}

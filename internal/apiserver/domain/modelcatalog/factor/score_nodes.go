package factor

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

// CalculationScoreNodesFromSnapshots translates catalog factor snapshots to calculation score nodes.
func CalculationScoreNodesFromSnapshots(factors []FactorSnapshot) []calculation.ScoreNode {
	if len(factors) == 0 {
		return nil
	}
	inferred := InferParentCodesFromChildrenPolicy(factors)
	nodes := make([]calculation.ScoreNode, 0, len(inferred))
	for _, f := range inferred {
		role := f.ResolvedRole()
		node := calculation.ScoreNode{
			Code:       f.Code,
			Name:       f.Title,
			Role:       string(role),
			Kind:       calculationDimensionKindForRole(role),
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

// ValidateCalculationScoreNodes validates the score graph derived from catalog factors.
func ValidateCalculationScoreNodes(factors []FactorSnapshot) error {
	nodes := CalculationScoreNodesFromSnapshots(factors)
	issues := calculation.ValidateScoreNodes(nodes)
	if len(issues) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(issues))
	for _, issue := range issues {
		msgs = append(msgs, issue.Message)
	}
	return fmt.Errorf("invalid score node graph: %s", strings.Join(msgs, "; "))
}

func calculationDimensionKindForRole(role FactorRole) calculation.DimensionKind {
	if role.Resolved() == FactorRoleIndex {
		return calculation.DimensionKindIndex
	}
	return calculation.DimensionKindFactor
}

func aggregationFromChildrenStrategy(strategy ChildrenAggregationStrategy) calculation.AggregationStrategy {
	switch strategy {
	case ChildrenAggregationSum:
		return calculation.AggregationSum
	case ChildrenAggregationAverage:
		return calculation.AggregationAverage
	case ChildrenAggregationWeightedSum:
		return calculation.AggregationWeightedSum
	case ChildrenAggregationLookup:
		return calculation.AggregationLookup
	case ChildrenAggregationCustom:
		return calculation.AggregationCustom
	case ChildrenAggregationNone:
		return calculation.AggregationNone
	default:
		return ""
	}
}

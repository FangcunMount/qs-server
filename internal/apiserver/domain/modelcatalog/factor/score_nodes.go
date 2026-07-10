package factor

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

// CalculationScoreNodesFromMeasureParts translates measure-layer parts to calculation score nodes.
func CalculationScoreNodesFromMeasureParts(factors []Factor, graph FactorGraph, scoring []Scoring) []calculation.ScoreNode {
	if len(factors) == 0 {
		return nil
	}
	levels := graph.Levels()
	scoringByFactor := make(map[string]Scoring, len(scoring))
	for _, rule := range scoring {
		scoringByFactor[rule.FactorCode] = rule
	}
	nodes := make([]calculation.ScoreNode, 0, len(factors))
	for _, f := range factors {
		role := f.ResolvedRole()
		node := calculation.ScoreNode{
			Code:       f.Code,
			Name:       f.Title,
			Role:       string(role),
			Kind:       calculationDimensionKindForRole(role),
			ParentCode: graph.ParentCode(f.Code),
			Level:      levels[f.Code],
			SortOrder:  graph.SortOrders[f.Code],
		}
		if rule, ok := scoringByFactor[f.Code]; ok && scoringHasSourceKind(rule, ScoringSourceFactor) {
			node.Aggregation = aggregationFromChildrenStrategy(ChildrenAggregationStrategy(rule.Strategy))
			node.Children = scoringSourceCodes(rule.Sources)
			if len(rule.Weights) > 0 {
				node.Weights = make(map[string]float64, len(rule.Weights))
				for code, weight := range rule.Weights {
					node.Weights[code] = weight
				}
			}
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// ValidateCalculationScoreNodesFromMeasureParts validates the score graph derived from measure-layer parts.
func ValidateCalculationScoreNodesFromMeasureParts(factors []Factor, graph FactorGraph, scoring []Scoring) error {
	nodes := CalculationScoreNodesFromMeasureParts(factors, graph, scoring)
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

func scoringSourceCodes(sources []ScoringSource) []string {
	if len(sources) == 0 {
		return nil
	}
	codes := make([]string, 0, len(sources))
	for _, source := range sources {
		codes = append(codes, source.Code)
	}
	return codes
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

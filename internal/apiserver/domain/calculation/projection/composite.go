package projection

import (
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

// CompositeProjection 推导父节点/index 原始分 从 子节点 维度分。
type CompositeProjection struct {
	Nodes []calculation.ScoreNode
}

func (p CompositeProjection) Apply(result *calculation.Result) *calculation.Result {
	if result == nil || len(p.Nodes) == 0 {
		return result
	}
	composites := compositeNodes(p.Nodes)
	if len(composites) == 0 {
		return result
	}
	sort.Slice(composites, func(i, j int) bool {
		return composites[i].Level > composites[j].Level
	})

	scores := dimensionScoresByCode(result.Dimensions)
	for _, parent := range composites {
		raw, ok := aggregateChildScore(parent, scores)
		if !ok {
			continue
		}
		scores[parent.Code] = raw
		upsertDimensionScore(result, parent, raw)
	}
	return result
}

func compositeNodes(nodes []calculation.ScoreNode) []calculation.ScoreNode {
	out := make([]calculation.ScoreNode, 0)
	for _, node := range nodes {
		if len(node.Children) == 0 {
			continue
		}
		out = append(out, node)
	}
	return out
}

func dimensionScoresByCode(dimensions []calculation.DimensionResult) map[string]float64 {
	scores := make(map[string]float64, len(dimensions))
	for _, dim := range dimensions {
		if dim.Score == nil {
			continue
		}
		scores[dim.Code] = dim.Score.Value
	}
	return scores
}

func aggregateChildScore(node calculation.ScoreNode, scores map[string]float64) (float64, bool) {
	if len(node.Children) == 0 {
		return 0, false
	}
	switch node.Aggregation {
	case calculation.AggregationNone, calculation.AggregationLookup, calculation.AggregationCustom:
		return 0, false
	case calculation.AggregationAverage:
		return aggregateAverage(node.Children, scores)
	case calculation.AggregationWeightedSum:
		return aggregateWeightedSum(node.Children, node.Weights, scores)
	case calculation.AggregationSum:
		return aggregateSum(node.Children, scores)
	default:
		return 0, false
	}
}

func aggregateSum(children []string, scores map[string]float64) (float64, bool) {
	var sum float64
	var found bool
	for _, code := range children {
		score, ok := scores[code]
		if !ok {
			continue
		}
		sum += score
		found = true
	}
	return sum, found
}

// aggregate平均值 computes sum(存在 子节点 分数) / len(子节点)。
// Missing 子节点 contribute 0 到 分子 但 still count toward divisor,。
// so 缺失 分数 dilute 平均值 rather than being ignored in 分母。
func aggregateAverage(children []string, scores map[string]float64) (float64, bool) {
	sum, found := aggregateSum(children, scores)
	if !found {
		return 0, false
	}
	return sum / float64(len(children)), true
}

func aggregateWeightedSum(children []string, weights map[string]float64, scores map[string]float64) (float64, bool) {
	var sum float64
	var found bool
	for _, code := range children {
		score, ok := scores[code]
		if !ok {
			continue
		}
		weight := 1.0
		if weights != nil {
			if w, ok := weights[code]; ok {
				weight = w
			}
		}
		sum += score * weight
		found = true
	}
	return sum, found
}

func upsertDimensionScore(result *calculation.Result, parent calculation.ScoreNode, raw float64) {
	for i := range result.Dimensions {
		if result.Dimensions[i].Code != parent.Code {
			continue
		}
		applyNodeMetadata(&result.Dimensions[i], parent)
		result.Dimensions[i].Score = &calculation.ScoreValue{
			Kind:  calculation.ScoreKindRawTotal,
			Value: raw,
		}
		return
	}
	dim := calculation.DimensionResult{
		Code: parent.Code,
		Score: &calculation.ScoreValue{
			Kind:  calculation.ScoreKindRawTotal,
			Value: raw,
		},
	}
	applyNodeMetadata(&dim, parent)
	result.Dimensions = append(result.Dimensions, dim)
}

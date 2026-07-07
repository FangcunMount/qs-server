package projection

import (
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// CompositeProjection derives parent/index raw scores from child dimension scores.
type CompositeProjection struct {
	Factors []factor.FactorSnapshot
}

func (p CompositeProjection) Apply(outcome *assessment.AssessmentOutcome) *assessment.AssessmentOutcome {
	if outcome == nil || len(p.Factors) == 0 {
		return outcome
	}
	composites := compositeFactors(p.Factors)
	if len(composites) == 0 {
		return outcome
	}
	sort.Slice(composites, func(i, j int) bool {
		return composites[i].Level > composites[j].Level
	})

	scores := dimensionScoresByCode(outcome.Dimensions)
	for _, parent := range composites {
		if parent.ChildrenPolicy == nil {
			continue
		}
		raw, ok := aggregateChildScore(parent.ChildrenPolicy, scores)
		if !ok {
			continue
		}
		scores[parent.Code] = raw
		upsertDimensionScore(outcome, parent, raw)
	}
	return outcome
}

func compositeFactors(factors []factor.FactorSnapshot) []factor.FactorSnapshot {
	out := make([]factor.FactorSnapshot, 0)
	for _, item := range factor.InferParentCodesFromChildrenPolicy(factors) {
		if item.ChildrenPolicy == nil {
			continue
		}
		out = append(out, item)
	}
	return out
}

func dimensionScoresByCode(dimensions []assessment.DimensionResult) map[string]float64 {
	scores := make(map[string]float64, len(dimensions))
	for _, dim := range dimensions {
		if dim.Score == nil {
			continue
		}
		scores[dim.Code] = dim.Score.Value
	}
	return scores
}

func aggregateChildScore(policy *factor.ChildrenPolicy, scores map[string]float64) (float64, bool) {
	if policy == nil || len(policy.Children) == 0 {
		return 0, false
	}
	switch policy.Strategy {
	case factor.ChildrenAggregationNone, factor.ChildrenAggregationLookup, factor.ChildrenAggregationCustom:
		return 0, false
	case factor.ChildrenAggregationAverage:
		return aggregateAverage(policy.Children, scores)
	case factor.ChildrenAggregationWeightedSum:
		return aggregateWeightedSum(policy.Children, policy.Weights, scores)
	case factor.ChildrenAggregationSum, "":
		fallthrough
	default:
		return aggregateSum(policy.Children, scores)
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

func upsertDimensionScore(outcome *assessment.AssessmentOutcome, parent factor.FactorSnapshot, raw float64) {
	for i := range outcome.Dimensions {
		if outcome.Dimensions[i].Code != parent.Code {
			continue
		}
		applyFactorMetadata(&outcome.Dimensions[i], parent)
		outcome.Dimensions[i].Score = &assessment.OutcomeScoreValue{
			Kind:  assessment.OutcomeScoreKindRawTotal,
			Value: raw,
		}
		return
	}
	dim := assessment.DimensionResult{
		Code: parent.Code,
		Score: &assessment.OutcomeScoreValue{
			Kind:  assessment.OutcomeScoreKindRawTotal,
			Value: raw,
		},
	}
	applyFactorMetadata(&dim, parent)
	outcome.Dimensions = append(outcome.Dimensions, dim)
}

func dimensionKindForRole(role factor.FactorRole) assessment.DimensionKind {
	switch role.Resolved() {
	case factor.FactorRoleIndex:
		return assessment.DimensionKindIndex
	default:
		return assessment.DimensionKindFactor
	}
}

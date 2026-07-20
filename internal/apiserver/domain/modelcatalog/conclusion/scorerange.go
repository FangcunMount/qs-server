package conclusion

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"

// ScoreRangeBound is the ModelCatalog view of the shared calculation scorerange.Bound.
type ScoreRangeBound = scorerange.Bound

// Bound returns the endpoint contract for a ScoreRangeOutcome.
func (r ScoreRangeOutcome) Bound() ScoreRangeBound {
	return ScoreRangeBound{
		Min:          r.MinScore,
		Max:          r.MaxScore,
		MaxInclusive: r.MaxInclusive,
		UnboundedMax: r.UnboundedMax,
	}
}

// HasExplicitEndpointSemantics is true when any rule declares MaxInclusive or UnboundedMax.
func HasExplicitEndpointSemantics(rules []ScoreRangeOutcome) bool {
	for _, rule := range rules {
		if rule.MaxInclusive || rule.UnboundedMax {
			return true
		}
	}
	return false
}

// MatchScoreRangeOutcomes returns the first matching rule using the shared endpoint contract.
func MatchScoreRangeOutcomes(score float64, rules []ScoreRangeOutcome) (ScoreRangeOutcome, bool) {
	if len(rules) == 0 {
		return ScoreRangeOutcome{}, false
	}
	bounds := make([]ScoreRangeBound, len(rules))
	for i := range rules {
		bounds[i] = rules[i].Bound()
	}
	index, ok := MatchBounds(score, bounds)
	if !ok {
		return ScoreRangeOutcome{}, false
	}
	return rules[index], true
}

// MatchBounds delegates to the calculation-kernel matcher.
func MatchBounds(score float64, bounds []ScoreRangeBound) (int, bool) {
	return scorerange.MatchBounds(score, bounds)
}

// RangesOverlap delegates to the calculation-kernel helper.
func RangesOverlap(a, b ScoreRangeBound) bool {
	return scorerange.RangesOverlap(a, b)
}

// HasCoverageGap delegates to the calculation-kernel helper.
func HasCoverageGap(left, right ScoreRangeBound) bool {
	return scorerange.HasCoverageGap(left, right)
}

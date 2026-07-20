// Package scorerange provides the neutral score-range endpoint contract used by
// the calculation kernel and by ModelCatalog Decision validation/matching.
package scorerange

// Bound is the shared endpoint contract for score ranges.
// Default (both flags false) is half-open [Min, Max).
// MaxInclusive makes the range closed on the right: [Min, Max].
// UnboundedMax ignores Max and matches any score >= Min.
type Bound struct {
	Min          float64
	Max          float64
	MaxInclusive bool
	UnboundedMax bool
}

// Contains reports whether score falls in the bound.
func (b Bound) Contains(score float64) bool {
	if score < b.Min {
		return false
	}
	if b.UnboundedMax {
		return true
	}
	if b.MaxInclusive {
		return score <= b.Max
	}
	return score < b.Max
}

// MatchBounds returns the index of the first matching bound.
// When no bound declares MaxInclusive or UnboundedMax, the last bound is
// treated as max-inclusive for historical snapshot compatibility.
func MatchBounds(score float64, bounds []Bound) (int, bool) {
	if len(bounds) == 0 {
		return -1, false
	}
	legacyLastInclusive := true
	for _, bound := range bounds {
		if bound.MaxInclusive || bound.UnboundedMax {
			legacyLastInclusive = false
			break
		}
	}
	for i, bound := range bounds {
		candidate := bound
		if legacyLastInclusive && i == len(bounds)-1 {
			candidate.MaxInclusive = true
		}
		if candidate.Contains(score) {
			return i, true
		}
	}
	return -1, false
}

// EntirelyLeft reports whether left ends strictly before right starts.
func EntirelyLeft(left, right Bound) bool {
	if left.UnboundedMax {
		return false
	}
	if left.MaxInclusive {
		return left.Max < right.Min
	}
	return left.Max <= right.Min
}

// RangesOverlap reports whether two bounds share any score.
func RangesOverlap(a, b Bound) bool {
	return !EntirelyLeft(a, b) && !EntirelyLeft(b, a)
}

// HasCoverageGap reports a coverage gap between sorted neighbors.
func HasCoverageGap(left, right Bound) bool {
	if left.UnboundedMax {
		return false
	}
	if left.MaxInclusive {
		return left.Max < right.Min
	}
	return left.Max < right.Min
}

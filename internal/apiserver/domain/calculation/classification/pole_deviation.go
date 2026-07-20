package classification

// PoleMaxDeviation computes the maximum absolute deviation from threshold
// across the possible raw score range of a pole leaf factor.
// When threshold is 0, the historical default 24 is used.
// Contributions without OptionScores assume a 1..5 Likert option span.
func PoleMaxDeviation(constant, threshold float64, contributions []AnswerContribution) float64 {
	minScore := constant
	maxScore := constant
	for _, contribution := range contributions {
		if len(contribution.OptionScores) > 0 {
			var localMin, localMax float64
			first := true
			for _, score := range contribution.OptionScores {
				if first {
					localMin, localMax = score, score
					first = false
					continue
				}
				if score < localMin {
					localMin = score
				}
				if score > localMax {
					localMax = score
				}
			}
			minScore += localMin
			maxScore += localMax
			continue
		}
		sign := contribution.Sign
		if sign > 0 {
			minScore += sign * 1
			maxScore += sign * 5
		} else {
			minScore += sign * 5
			maxScore += sign * 1
		}
	}
	if threshold == 0 {
		threshold = 24
	}
	left := threshold - minScore
	right := maxScore - threshold
	if left > right {
		return left
	}
	return right
}

package classification

// DualScaleFromScore expands a single match score into (matchPercent, similarity).
// Heuristic (historical typology behavior):
//   - score in (0, 1]: treated as similarity → matchPercent = score * 100
//   - score > 1: treated as matchPercent → similarity = score / 100
//   - otherwise (incl. 0): both fields keep the input value
func DualScaleFromScore(score float64) (matchPercent, similarity float64) {
	matchPercent, similarity = score, score
	if score > 0 && score <= 1 {
		matchPercent = score * 100
	} else if score > 1 {
		similarity = score / 100
	}
	return matchPercent, similarity
}

// MatchPercentPrefer returns matchPercent when set; otherwise converts similarity
// (0–1 scale) to percent. Used when assembling Outcome primary scores.
func MatchPercentPrefer(matchPercent, similarity float64) float64 {
	if matchPercent == 0 && similarity > 0 {
		return similarity * 100
	}
	return matchPercent
}

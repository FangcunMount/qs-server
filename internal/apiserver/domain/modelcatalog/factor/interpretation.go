package factor

// ScoreRangeRule maps a raw score interval to an interpretation outcome.
type ScoreRangeRule struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	Level      string  `json:"level,omitempty"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion,omitempty"`
}

// Matches reports whether score falls in [MinScore, MaxScore).
func (r ScoreRangeRule) Matches(score float64) bool {
	return score >= r.MinScore && score < r.MaxScore
}

// InterpretationSpec groups score-range rules for scoring models.
type InterpretationSpec struct {
	Ranges []ScoreRangeRule
}

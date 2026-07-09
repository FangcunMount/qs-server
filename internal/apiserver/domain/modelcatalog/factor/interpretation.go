package factor

// ScoreRangeRule 映射原始分 interval 到 interpretation 结果。
type ScoreRangeRule struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	Level      string  `json:"level,omitempty"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion,omitempty"`
}

// Matches 报告是否 score falls in [MinScore, MaxScore)。
func (r ScoreRangeRule) Matches(score float64) bool {
	return score >= r.MinScore && score < r.MaxScore
}

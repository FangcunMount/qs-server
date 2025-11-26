package request

// UpdateScoreRequest 更新分数请求
type UpdateScoreRequest struct {
	AnswerSheetID uint64             `json:"answersheet_id" valid:"required"`
	AnswerScores  map[string]float64 `json:"answer_scores" valid:"required"`
}

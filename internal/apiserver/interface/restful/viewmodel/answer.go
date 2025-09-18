package viewmodel

// AnswerDTO 答案
type AnswerDTO struct {
	QuestionCode string  `json:"question_code" valid:"required"`
	QuestionType string  `json:"question_type" valid:"required"`
	Value        any     `json:"value"`
	Score        float64 `json:"score"`
}

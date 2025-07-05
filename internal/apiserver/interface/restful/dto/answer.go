package dto

// Answer 答案
type Answer struct {
	QuestionCode string `json:"question_code" valid:"required"`
	QuestionType string `json:"question_type" valid:"required"`
	Score        uint16 `json:"score"`
	Value        any    `json:"value"`
}

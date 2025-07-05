package dto

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer"
	answer_values "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer/answer-values"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
)

// Answer 答案
type Answer struct {
	QuestionCode string `json:"question_code" valid:"required"`
	QuestionType string `json:"question_type" valid:"required"`
	Value        any    `json:"value"`
	Score        uint16 `json:"score"`
}

// AnswerMapper 答案映射
type AnswerMapper struct{}

func NewAnswerMapper() *AnswerMapper {
	return &AnswerMapper{}
}

// MapAnswersToBOs 将 Answer 列表转为领域对象列表
func (m *AnswerMapper) MapAnswersToBOs(answers []Answer) []answer.Answer {
	domainAnswers := make([]answer.Answer, len(answers))
	for i, a := range answers {
		domainAnswers[i] = answer.NewAnswer(
			a.QuestionCode,
			a.QuestionType,
			0,
			answer_values.NewAnswerValue(question.QuestionType(a.QuestionType), a.Value),
		)
	}

	return domainAnswers
}

// MapAnswersToDTOs 将领域对象列表转为 Answer 列表
func (m *AnswerMapper) MapAnswersToDTOs(answers []*answer.Answer) []Answer {
	dtos := make([]Answer, len(answers))
	for i, a := range answers {
		dtos[i] = Answer{
			QuestionCode: a.GetQuestionCode(),
			QuestionType: a.GetQuestionType(),
			Value:        a.GetValue().Raw(),
		}
	}
	return dtos
}

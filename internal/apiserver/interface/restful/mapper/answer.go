package mapper

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer"
	answer_values "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer/answer-values"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"
)

// AnswerMapper 答案映射
type AnswerMapper struct{}

func NewAnswerMapper() *AnswerMapper {
	return &AnswerMapper{}
}

// MapAnswersToBOs 将 Answer 列表转为领域对象列表
func (m *AnswerMapper) MapAnswersToBOs(answers []viewmodel.AnswerDTO) []answer.Answer {
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
func (m *AnswerMapper) MapAnswersToDTOs(answers []*answer.Answer) []viewmodel.AnswerDTO {
	dtos := make([]viewmodel.AnswerDTO, len(answers))
	for i, a := range answers {
		dtos[i] = viewmodel.AnswerDTO{
			QuestionCode: a.GetQuestionCode(),
			QuestionType: a.GetQuestionType(),
			Value:        a.GetValue().Raw(),
		}
	}
	return dtos
}

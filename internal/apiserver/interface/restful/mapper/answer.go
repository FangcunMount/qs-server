package mapper

import (
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/answersheet/answer"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire/question"
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/viewmodel"
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
		domainAnswers[i], _ = answer.NewAnswer(
			question.NewQuestionCode(a.QuestionCode),
			question.QuestionType(a.QuestionType),
			0,
			a.Value,
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

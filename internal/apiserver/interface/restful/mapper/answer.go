package mapper

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/viewmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AnswerMapper 答案映射
type AnswerMapper struct{}

func NewAnswerMapper() *AnswerMapper {
	return &AnswerMapper{}
}

// MapAnswersToBOs 将 Answer 列表转为领域对象列表
func (m *AnswerMapper) MapAnswersToBOs(answers []viewmodel.AnswerDTO) ([]answersheet.Answer, error) {
	domainAnswers := make([]answersheet.Answer, 0, len(answers))
	for _, a := range answers {
		qType := questionnaire.QuestionType(a.QuestionType)

		// 根据问题类型创建答案值
		answerValue, err := answersheet.CreateAnswerValueFromRaw(qType, a.Value)
		if err != nil {
			return nil, err
		}

		ans, err := answersheet.NewAnswer(
			meta.NewCode(a.QuestionCode),
			qType,
			answerValue,
			0, // 初始分数为0
		)
		if err != nil {
			return nil, err
		}

		domainAnswers = append(domainAnswers, ans)
	}

	return domainAnswers, nil
}

// MapAnswersToDTOs 将领域对象列表转为 Answer 列表
func (m *AnswerMapper) MapAnswersToDTOs(answers []answersheet.Answer) []viewmodel.AnswerDTO {
	dtos := make([]viewmodel.AnswerDTO, len(answers))
	for i, a := range answers {
		dtos[i] = viewmodel.AnswerDTO{
			QuestionCode: a.QuestionCode(),
			QuestionType: a.QuestionType(),
			Value:        a.Value().Raw(),
		}
	}
	return dtos
}

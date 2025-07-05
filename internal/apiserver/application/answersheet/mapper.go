package answersheet

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer"
	answer_values "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer/answer-values"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
)

// AnswerMapper DTO 与领域对象转换器
type AnswerMapper struct{}

// NewAnswerMapper 创建答案映射器
func NewAnswerMapper() *AnswerMapper {
	return &AnswerMapper{}
}

// ToDTO 将领域对象转换为 DTO
func (m *AnswerMapper) ToDTO(bo *answer.Answer) *dto.AnswerDTO {
	if bo == nil {
		return nil
	}

	return &dto.AnswerDTO{
		QuestionCode: bo.GetQuestionCode(),
		QuestionType: bo.GetQuestionType(),
		Score:        bo.GetScore(),
		Value:        bo.GetValue().Raw(),
	}
}

// ToDTOs 将领域对象列表转换为 DTO 列表
func (m *AnswerMapper) ToDTOs(bos []answer.Answer) []dto.AnswerDTO {
	dtos := make([]dto.AnswerDTO, len(bos))
	for i, bo := range bos {
		dtos[i] = *m.ToDTO(&bo)
	}
	return dtos
}

// ToBO 将 DTO 转换为领域对象
func (m *AnswerMapper) ToBO(dto *dto.AnswerDTO) answer.Answer {
	if dto == nil {
		return answer.Answer{}
	}

	questionType := question.QuestionType(dto.QuestionType)
	answerValue := answer_values.NewAnswerValue(questionType, dto.Value)

	return answer.NewAnswer(
		dto.QuestionCode,
		dto.QuestionType,
		dto.Score,
		answerValue,
	)
}

// ToBOs 将 DTO 列表转换为领域对象列表
func (m *AnswerMapper) ToBOs(dtos []dto.AnswerDTO) []answer.Answer {
	bos := make([]answer.Answer, len(dtos))
	for i, dto := range dtos {
		bos[i] = m.ToBO(&dto)
	}
	return bos
}

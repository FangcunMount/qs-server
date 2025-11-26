package mapper

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AnswerMapper DTO 与领域对象转换器
type AnswerMapper struct{}

// NewAnswerMapper 创建答案映射器
func NewAnswerMapper() AnswerMapper {
	return AnswerMapper{}
}

// ToDTO 将领域对象转换为 DTO
func (m *AnswerMapper) ToDTO(bo answersheet.Answer) *dto.AnswerDTO {
	return &dto.AnswerDTO{
		QuestionCode: bo.QuestionCode(),
		QuestionType: bo.QuestionType(),
		Score:        bo.Score(),
		Value:        bo.Value().Raw(),
	}
}

// ToDTOs 将领域对象列表转换为 DTO 列表
func (m *AnswerMapper) ToDTOs(bos []answersheet.Answer) []dto.AnswerDTO {
	if bos == nil {
		return []dto.AnswerDTO{} // 返回空切片而不是 nil
	}

	dtos := make([]dto.AnswerDTO, len(bos))
	for i, bo := range bos {
		dtos[i] = dto.AnswerDTO{
			QuestionCode: bo.QuestionCode(),
			QuestionType: bo.QuestionType(),
			Score:        bo.Score(),
			Value:        bo.Value().Raw(),
		}
	}
	return dtos
}

// ToBO 将 DTO 转换为领域对象
func (m *AnswerMapper) ToBO(dto *dto.AnswerDTO) (answersheet.Answer, error) {
	if dto == nil {
		return answersheet.Answer{}, nil
	}

	qType := questionnaire.QuestionType(dto.QuestionType)

	// 根据问题类型创建答案值
	answerValue, err := answersheet.CreateAnswerValueFromRaw(qType, dto.Value)
	if err != nil {
		return answersheet.Answer{}, err
	}

	return answersheet.NewAnswer(
		meta.NewCode(dto.QuestionCode),
		qType,
		answerValue,
		dto.Score,
	)
}

// ToBOs 将 DTO 列表转换为领域对象列表
func (m *AnswerMapper) ToBOs(dtos []dto.AnswerDTO) ([]answersheet.Answer, error) {
	bos := make([]answersheet.Answer, 0, len(dtos))
	for _, dto := range dtos {
		bo, err := m.ToBO(&dto)
		if err != nil {
			return nil, err
		}
		bos = append(bos, bo)
	}
	return bos, nil
}

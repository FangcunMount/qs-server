package questionnaire

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

type QuestionnaireMapper struct{}

func NewQuestionnaireMapper() *QuestionnaireMapper {
	return &QuestionnaireMapper{}
}

// ToEntity 将领域模型转换为实体
func (m *QuestionnaireMapper) ToEntity(domainQuestionnaire *questionnaire.Questionnaire) *QuestionnaireEntity {
	return &QuestionnaireEntity{
		Code:    domainQuestionnaire.Code,
		Title:   domainQuestionnaire.Title,
		ImgUrl:  domainQuestionnaire.ImgUrl,
		Version: domainQuestionnaire.Version,
		Status:  domainQuestionnaire.Status,
	}
}

// ToDomain 将实体转换为领域模型
func (m *QuestionnaireMapper) ToDomain(entity *QuestionnaireEntity) *questionnaire.Questionnaire {
	return &questionnaire.Questionnaire{
		Code:    entity.Code,
		Title:   entity.Title,
		ImgUrl:  entity.ImgUrl,
		Version: entity.Version,
		Status:  entity.Status,
	}
}

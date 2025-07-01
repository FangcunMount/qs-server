package questionnaire

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

type QuestionnaireMapper struct{}

func NewQuestionnaireMapper() *QuestionnaireMapper {
	return &QuestionnaireMapper{}
}

// ToPO 将领域模型转换为持久化对象
func (m *QuestionnaireMapper) ToPO(domainQuestionnaire *questionnaire.Questionnaire) *QuestionnairePO {
	return &QuestionnairePO{
		Code:        domainQuestionnaire.Code,
		Title:       domainQuestionnaire.Title,
		Description: domainQuestionnaire.Description,
		ImgUrl:      domainQuestionnaire.ImgUrl,
		Version:     domainQuestionnaire.Version,
		Status:      domainQuestionnaire.Status,
	}
}

// ToBO 将持久化对象转换为业务对象
func (m *QuestionnaireMapper) ToBO(po *QuestionnairePO) *questionnaire.Questionnaire {
	return &questionnaire.Questionnaire{
		Code:        po.Code,
		Title:       po.Title,
		Description: po.Description,
		ImgUrl:      po.ImgUrl,
		Version:     po.Version,
		Status:      po.Status,
	}
}

package questionnaire

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

type QuestionnaireMapper struct{}

func NewQuestionnaireMapper() *QuestionnaireMapper {
	return &QuestionnaireMapper{}
}

// ToPO 将领域模型转换为持久化对象
func (m *QuestionnaireMapper) ToPO(bo *questionnaire.Questionnaire) *QuestionnairePO {
	return &QuestionnairePO{
		Code:        bo.GetCode().Value(),
		Title:       bo.GetTitle(),
		Description: bo.GetDescription(),
		ImgUrl:      bo.GetImgUrl(),
		Version:     bo.GetVersion().Value(),
		Status:      bo.GetStatus().Value(),
	}
}

// ToBO 将持久化对象转换为业务对象
func (m *QuestionnaireMapper) ToBO(po *QuestionnairePO) *questionnaire.Questionnaire {
	return questionnaire.NewQuestionnaire(
		questionnaire.NewQuestionnaireCode(po.Code),
		questionnaire.WithTitle(po.Title),
		questionnaire.WithDescription(po.Description),
		questionnaire.WithImgUrl(po.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewQuestionnaireVersion(po.Version)),
		questionnaire.WithStatus(questionnaire.QuestionnaireStatus(po.Status)),
	)
}

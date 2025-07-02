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
	po := &QuestionnairePO{
		ID:          bo.GetID().Value(),
		Code:        bo.GetCode().Value(),
		Title:       bo.GetTitle(),
		Description: bo.GetDescription(),
		ImgUrl:      bo.GetImgUrl(),
		Version:     bo.GetVersion().Value(),
		Status:      bo.GetStatus().Value(),
	}

	return po
}

// ToBO 将持久化对象转换为业务对象
func (m *QuestionnaireMapper) ToBO(po *QuestionnairePO) *questionnaire.Questionnaire {
	q := questionnaire.NewQuestionnaire(
		questionnaire.NewQuestionnaireCode(po.Code),
		questionnaire.WithID(questionnaire.NewQuestionnaireID(po.ID)),
		questionnaire.WithTitle(po.Title),
		questionnaire.WithDescription(po.Description),
		questionnaire.WithImgUrl(po.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewQuestionnaireVersion(po.Version)),
		questionnaire.WithStatus(questionnaire.QuestionnaireStatus(po.Status)),
	)

	return q
}

// ToBOList 将持久化对象列表转换为业务对象列表
func (m *QuestionnaireMapper) ToBOList(pos []*QuestionnairePO) []*questionnaire.Questionnaire {
	bos := make([]*questionnaire.Questionnaire, len(pos))
	for i, po := range pos {
		bos[i] = m.ToBO(po)
	}
	return bos
}

package questionnaire

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type QuestionnaireMapper struct{}

func NewQuestionnaireMapper() *QuestionnaireMapper {
	return &QuestionnaireMapper{}
}

// ToPO 将领域模型转换为持久化对象
func (m *QuestionnaireMapper) ToPO(bo *questionnaire.Questionnaire) *QuestionnairePO {
	po := &QuestionnairePO{
		Code:        bo.GetCode().Value(),
		Title:       bo.GetTitle(),
		Description: bo.GetDescription(),
		ImgUrl:      bo.GetImgUrl(),
		Version:     bo.GetVersion().Value(),
		Status:      bo.GetStatus().Value(),
	}

	// 设置 AuditFields 中的 ID
	po.AuditFields.ID = bo.GetID()

	return po
}

// ToBO 将持久化对象转换为业务对象
func (m *QuestionnaireMapper) ToBO(po *QuestionnairePO) *questionnaire.Questionnaire {
	qBO := questionnaire.NewQuestionnaire(
		meta.NewCode(po.Code),
		po.Title,
		questionnaire.WithID(po.AuditFields.ID),
		questionnaire.WithDescription(po.Description),
		questionnaire.WithImgUrl(po.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewQuestionnaireVersion(po.Version)),
		questionnaire.WithStatus(questionnaire.QuestionnaireStatus(po.Status)),
	)

	return qBO
}

// ToBOList 将持久化对象列表转换为业务对象列表
func (m *QuestionnaireMapper) ToBOList(pos []*QuestionnairePO) []*questionnaire.Questionnaire {
	bos := make([]*questionnaire.Questionnaire, len(pos))
	for i, po := range pos {
		bos[i] = m.ToBO(po)
	}
	return bos
}

package answersheet

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AnswerSheetMapper 答卷映射器
type AnswerSheetMapper struct{}

// NewAnswerSheetMapper 创建答卷映射器
func NewAnswerSheetMapper() *AnswerSheetMapper {
	return &AnswerSheetMapper{}
}

// ToPO 将领域模型转换为MongoDB持久化对象
func (m *AnswerSheetMapper) ToPO(bo *answersheet.AnswerSheet) *AnswerSheetPO {
	if bo == nil {
		return nil
	}

	// 转换答案
	answers := make([]AnswerPO, 0, len(bo.Answers()))
	for _, answer := range bo.Answers() {
		if po := m.mapAnswerToPO(answer); po != nil {
			answers = append(answers, *po)
		}
	}

	// 获取问卷信息
	code, version, title := bo.QuestionnaireInfo()

	// 获取填写者信息
	filler := bo.Filler()
	var fillerID int64
	var fillerType string
	if filler != nil {
		fillerID = filler.UserID()
		fillerType = string(filler.FillerType())
	}

	// 创建PO对象
	po := &AnswerSheetPO{
		QuestionnaireCode:    code,
		QuestionnaireVersion: version,
		QuestionnaireTitle:   title,
		FillerID:             fillerID,
		FillerType:           fillerType,
		TotalScore:           bo.Score(),
		FilledAt:             bo.FilledAt(),
		Answers:              answers,
	}

	// 如果领域对象有ID，则设置DomainID
	if !bo.ID().IsZero() {
		po.DomainID = bo.ID()
	}

	return po
}

// ToBO 将MongoDB持久化对象转换为业务对象
func (m *AnswerSheetMapper) ToBO(po *AnswerSheetPO) *answersheet.AnswerSheet {
	if po == nil {
		return nil
	}

	// 转换答案
	answers := make([]answersheet.Answer, 0, len(po.Answers))
	for _, answerPO := range po.Answers {
		answer, err := m.mapAnswerToBO(answerPO)
		if err != nil {
			// 如果答案转换失败，跳过该答案
			continue
		}
		answers = append(answers, answer)
	}

	// 构建问卷引用
	questionnaireRef := answersheet.NewQuestionnaireRef(
		po.QuestionnaireCode,
		po.QuestionnaireVersion,
		po.QuestionnaireTitle,
	)

	// 构建填写者引用
	filler := actor.NewFillerRef(po.FillerID, actor.FillerType(po.FillerType))

	// 使用 Reconstruct 重建答卷对象
	return answersheet.Reconstruct(
		po.DomainID,
		questionnaireRef,
		filler,
		answers,
		po.FilledAt,
		po.TotalScore,
	)
}

// mapAnswerToPO 将答案领域对象转换为 AnswerPO
func (m *AnswerSheetMapper) mapAnswerToPO(answerBO answersheet.Answer) *AnswerPO {
	return &AnswerPO{
		QuestionCode: answerBO.QuestionCode(),
		QuestionType: answerBO.QuestionType(),
		Score:        answerBO.Score(),
		Value: AnswerValuePO{
			Value: answerBO.Value().Raw(),
		},
	}
}

// mapAnswerToBO 将 AnswerPO 转换为答案领域对象
func (m *AnswerSheetMapper) mapAnswerToBO(answerPO AnswerPO) (answersheet.Answer, error) {
	// 创建答案值
	answerValue, err := answersheet.CreateAnswerValueFromRaw(
		questionnaire.QuestionType(answerPO.QuestionType),
		answerPO.Value.Value,
	)
	if err != nil {
		return answersheet.Answer{}, err
	}

	// 创建答案对象（带分数）
	answer, err := answersheet.NewAnswer(
		meta.NewCode(answerPO.QuestionCode),
		questionnaire.QuestionType(answerPO.QuestionType),
		answerValue,
		answerPO.Score,
	)
	if err != nil {
		return answersheet.Answer{}, err
	}

	return answer, nil
}

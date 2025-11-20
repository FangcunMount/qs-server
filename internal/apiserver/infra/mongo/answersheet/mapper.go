package answersheet

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet/answer"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/question"

	// TODO: 重构 - 使用 actor.FillerRef 和 actor.TesteeRef
	// "github.com/FangcunMount/qs-server/internal/apiserver/domain/user"
	// "github.com/FangcunMount/qs-server/internal/apiserver/domain/user/role"
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
	answers := make([]AnswerPO, 0, len(bo.GetAnswers()))
	for _, answer := range bo.GetAnswers() {
		if po := m.mapAnswerToPO(answer); po != nil {
			answers = append(answers, *po)
		}
	}

	// TODO: 重构 - 使用 actor.FillerRef
	// 临时注释掉
	var writer *WriterPO
	// if bo.GetWriter() != nil {
	// 	writer = &WriterPO{
	// 		UserID: bo.GetWriter().GetUserID().Uint64(),
	// 	}
	// }

	// TODO: 重构 - 使用 actor.TesteeRef
	// 临时注释掉
	var testee *TesteePO
	// if bo.GetTestee() != nil {
	// 	testee = &TesteePO{
	// 		UserID: bo.GetTestee().GetUserID().Uint64(),
	// 		Name:   bo.GetTestee().GetName(),
	// 		Sex:    bo.GetTestee().GetSex(),
	// 		Age:    bo.GetTestee().GetAge(),
	// 	}
	// }

	// 创建PO对象，但不设置DomainID，让BeforeInsert方法来设置
	po := &AnswerSheetPO{
		QuestionnaireCode:    bo.GetQuestionnaireCode(),
		QuestionnaireVersion: bo.GetQuestionnaireVersion(),
		Title:                bo.GetTitle(),
		Score:                bo.GetScore(),
		Answers:              answers,
		Writer:               writer,
		Testee:               testee,
	}

	// 设置时间字段
	po.CreatedAt = bo.GetCreatedAt()
	po.UpdatedAt = bo.GetUpdatedAt()

	// 如果领域对象有ID，则设置DomainID
	if !bo.GetID().IsZero() {
		po.DomainID = bo.GetID()
	}

	return po
}

// ToBO 将MongoDB持久化对象转换为业务对象
func (m *AnswerSheetMapper) ToBO(po *AnswerSheetPO) *answersheet.AnswerSheet {
	if po == nil {
		return nil
	}

	// 转换答案
	answers := make([]answer.Answer, 0, len(po.Answers))
	for _, answerPO := range po.Answers {
		answers = append(answers, m.mapAnswerToBO(answerPO))
	}

	// TODO: 重构 - 使用 actor.FillerRef 和 actor.TesteeRef
	// 临时注释掉，直接传 nil
	var writer interface{}
	var testee interface{}

	return answersheet.NewAnswerSheet(
		po.QuestionnaireCode,
		po.QuestionnaireVersion,
		answersheet.WithID(meta.ID(po.DomainID)),
		answersheet.WithTitle(po.Title),
		answersheet.WithScore(po.Score),
		answersheet.WithAnswers(answers),
		answersheet.WithWriter(writer),
		answersheet.WithTestee(testee),
		answersheet.WithCreatedAt(po.CreatedAt),
		answersheet.WithUpdatedAt(po.UpdatedAt),
	)
}

// mapAnswerToPO 将答案领域对象转换为 AnswerPO
func (m *AnswerSheetMapper) mapAnswerToPO(answerBO answer.Answer) *AnswerPO {
	return &AnswerPO{
		QuestionCode: answerBO.GetQuestionCode(),
		QuestionType: answerBO.GetQuestionType(),
		Score:        answerBO.GetScore(),
		Value: AnswerValuePO{
			Value: answerBO.GetValue().Raw(),
		},
	}
}

// mapAnswerToBO 将 AnswerPO 转换为答案领域对象
func (m *AnswerSheetMapper) mapAnswerToBO(answerPO AnswerPO) answer.Answer {
	ans, _ := answer.NewAnswer(
		meta.NewCode(answerPO.QuestionCode),
		question.QuestionType(answerPO.QuestionType),
		answerPO.Score,
		answerPO.Value.Value,
	)
	return ans
}

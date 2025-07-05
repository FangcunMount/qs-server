package answersheet

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer"
	answer_values "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer/answer-values"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo"
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
	for _, answerBO := range bo.GetAnswers() {
		answers = append(answers, *m.mapAnswerToPO(&answerBO))
	}

	// 转换答卷者 - 只存储 userID
	var writer *WriterPO
	if bo.GetWriter() != nil {
		writer = &WriterPO{
			UserID: bo.GetWriter().GetUserID().Value(),
		}
	}

	// 转换被试者 - 只存储 userID
	var testee *TesteePO
	if bo.GetTestee() != nil {
		testee = &TesteePO{
			UserID: bo.GetTestee().GetUserID().Value(),
		}
	}

	return &AnswerSheetPO{
		QuestionnaireCode:    bo.GetQuestionnaireCode(),
		QuestionnaireVersion: bo.GetQuestionnaireVersion(),
		Title:                bo.GetTitle(),
		Score:                bo.GetScore(),
		Answers:              answers,
		Writer:               writer,
		Testee:               testee,
	}
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

	// 转换答卷者 - 只使用 userID 创建 Writer
	var writer *user.Writer
	if po.Writer != nil {
		writer = user.NewWriter(user.NewUserID(po.Writer.UserID), "") // 名称留空，需要时从用户服务获取
	}

	// 转换被试者 - 只使用 userID 创建 Testee
	var testee *user.Testee
	if po.Testee != nil {
		testee = user.NewTestee(user.NewUserID(po.Testee.UserID), "") // 名称留空，需要时从用户服务获取
	}

	// 转换 ObjectID 为 uint64
	id := mongo.ObjectIDToUint64(po.ID)

	return answersheet.NewAnswerSheet(
		po.QuestionnaireCode,
		po.QuestionnaireVersion,
		answersheet.WithID(id),
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
func (m *AnswerSheetMapper) mapAnswerToPO(answerBO *answer.Answer) *AnswerPO {
	if answerBO == nil {
		return nil
	}

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
	// 使用工厂函数创建 AnswerValue
	questionType := question.QuestionType(answerPO.QuestionType)
	answerValue := answer_values.NewAnswerValue(questionType, answerPO.Value.Value)

	return answer.NewAnswer(
		answerPO.QuestionCode,
		answerPO.QuestionType,
		answerPO.Score,
		answerValue,
	)
}

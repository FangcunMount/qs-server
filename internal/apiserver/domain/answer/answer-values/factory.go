package answer_values

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
)

// NewAnswerValue 创建答案值
func NewAnswerValue(questionType question.QuestionType, value any) answer.AnswerValue {
	switch questionType {
	case question.QuestionTypeNumber:
		return NumberValue{V: value.(int)}
	case question.QuestionTypeRadio:
		return OptionValue{Code: value.(string)}
	case question.QuestionTypeCheckbox:
		return OptionsValue{V: value.([]OptionValue)}
	case question.QuestionTypeText:
		return StringValue{V: value.(string)}
	case question.QuestionTypeTextarea:
		return StringValue{V: value.(string)}
	}
	return nil
}

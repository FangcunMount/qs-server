package answer_values

import (
	"strconv"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/answer"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// NewAnswerValue 创建答案值
func NewAnswerValue(questionType question.QuestionType, value any) answer.AnswerValue {
	if value == nil {
		log.Warnf("Answer value is nil for question type: %s", questionType)
		return nil
	}

	switch questionType {
	case question.QuestionTypeNumber:
		// 尝试安全地转换为数字
		switch v := value.(type) {
		case int:
			return NumberValue{V: v}
		case float64:
			return NumberValue{V: int(v)}
		case string:
			// 尝试解析字符串为数字
			if num, err := strconv.Atoi(v); err == nil {
				return NumberValue{V: num}
			}
			log.Warnf("Failed to parse number from string: %s", v)
			return nil
		default:
			log.Warnf("Unexpected type for number question: %T, value: %v", value, value)
			return nil
		}
	case question.QuestionTypeRadio:
		// 尝试安全地转换为字符串
		switch v := value.(type) {
		case string:
			return OptionValue{Code: v}
		default:
			log.Warnf("Unexpected type for radio question: %T, value: %v", value, value)
			return nil
		}
	case question.QuestionTypeCheckbox:
		// 尝试安全地转换为选项数组
		switch v := value.(type) {
		case []OptionValue:
			return OptionsValue{V: v}
		case []string:
			// 将字符串数组转换为选项数组
			options := make([]OptionValue, len(v))
			for i, code := range v {
				options[i] = OptionValue{Code: code}
			}
			return OptionsValue{V: options}
		default:
			log.Warnf("Unexpected type for checkbox question: %T, value: %v", value, value)
			return nil
		}
	case question.QuestionTypeText:
		// 尝试安全地转换为字符串
		switch v := value.(type) {
		case string:
			return StringValue{V: v}
		default:
			log.Warnf("Unexpected type for text question: %T, value: %v", value, value)
			return nil
		}
	case question.QuestionTypeTextarea:
		// 尝试安全地转换为字符串
		switch v := value.(type) {
		case string:
			return StringValue{V: v}
		default:
			log.Warnf("Unexpected type for textarea question: %T, value: %v", value, value)
			return nil
		}
	default:
		log.Warnf("Unknown question type: %s", questionType)
		return nil
	}
}

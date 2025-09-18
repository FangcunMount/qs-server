package values

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/answer"
)

// 注册选项值工厂
func init() {
	answer.RegisterAnswerValueFactory(answer.OptionValueType, func(value any) answer.AnswerValue {
		if str, ok := value.(string); ok {
			return OptionValue{Code: str}
		}

		return nil
	})
}

// OptionValue 选项值
type OptionValue struct {
	Code string
}

// Raw 原始值
func (v OptionValue) Raw() any { return v.Code }

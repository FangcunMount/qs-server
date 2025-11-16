package values

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet/answer"

// 注册选项值工厂
func init() {
	answer.RegisterAnswerValueFactory(answer.StringValueType, func(value any) answer.AnswerValue {
		if str, ok := value.(string); ok {
			return StringValue{V: str}
		}

		return nil
	})
}

// StringValue 字符串值
type StringValue struct {
	V string
}

// Raw 原始值
func (v StringValue) Raw() any { return v.V }

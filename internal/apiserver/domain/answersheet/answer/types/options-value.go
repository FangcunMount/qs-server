package values

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet/answer"

// 注册选项值工厂
func init() {
	answer.RegisterAnswerValueFactory(answer.OptionsValueType, func(value any) answer.AnswerValue {
		switch v := value.(type) {
		case []string:
			// 处理字符串切片
			options := make([]OptionValue, len(v))
			for i, str := range v {
				options[i] = OptionValue{Code: str}
			}
			return OptionsValue{V: options}
		default:
			return nil
		}
	})
}

// OptionsValue 选项值
type OptionsValue struct {
	V []OptionValue
}

// Raw 原始值
func (v OptionsValue) Raw() any { return v.V }

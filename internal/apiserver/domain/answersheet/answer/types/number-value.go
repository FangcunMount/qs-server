package values

import (
	"strconv"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/answer"
)

// NumberValue 数值
type NumberValue struct {
	V float64
}

// 注册数值工厂
func init() {
	answer.RegisterAnswerValueFactory(answer.NumberValueType, func(value any) answer.AnswerValue {
		switch v := value.(type) {
		case int:
			return NumberValue{V: float64(v)}
		case float64:
			return NumberValue{V: v}
		case string:
			// 尝试将字符串解析为数字
			if num, err := strconv.Atoi(v); err == nil {
				return NumberValue{V: float64(num)}
			}
			return nil
		default:
			return nil
		}

	})
}

// Raw 原始值
func (v NumberValue) Raw() any { return v.V }

package answersheet

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/pkg/validation"
)

// AnswerValueAdapter 将 AnswerValue 适配为 ValidatableValue
// 这样可以让 answersheet 包中的 AnswerValue 被 validation 领域验证
type AnswerValueAdapter struct {
	answerValue AnswerValue
}

// NewAnswerValueAdapter 创建答案值适配器
func NewAnswerValueAdapter(value AnswerValue) validation.ValidatableValue {
	return &AnswerValueAdapter{answerValue: value}
}

// IsEmpty 值是否为空
func (a *AnswerValueAdapter) IsEmpty() bool {
	if a.answerValue == nil {
		return true
	}

	raw := a.answerValue.Raw()
	if raw == nil {
		return true
	}

	// 根据不同类型判断是否为空
	switch v := raw.(type) {
	case string:
		return v == ""
	case []string:
		return len(v) == 0
	case float64:
		return false // 数字类型，0也是有效值
	case int, int64:
		return false // 数字类型，0也是有效值
	default:
		return false
	}
}

// AsString 获取字符串表示
func (a *AnswerValueAdapter) AsString() string {
	if a.answerValue == nil {
		return ""
	}

	raw := a.answerValue.Raw()
	if raw == nil {
		return ""
	}

	// 根据不同类型转换为字符串
	switch v := raw.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case []string:
		// 多选值，返回第一个选项（如果需要校验所有选项，应该使用 AsArray）
		if len(v) > 0 {
			return v[0]
		}
		return ""
	default:
		return fmt.Sprintf("%v", raw)
	}
}

// AsNumber 获取数值表示
func (a *AnswerValueAdapter) AsNumber() (float64, error) {
	if a.answerValue == nil {
		return 0, fmt.Errorf("answer value is nil")
	}

	raw := a.answerValue.Raw()
	if raw == nil {
		return 0, fmt.Errorf("raw value is nil")
	}

	// 根据不同类型转换为数值
	switch v := raw.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		// 尝试解析字符串为数字
		num, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string '%s' to number: %w", v, err)
		}
		return num, nil
	default:
		return 0, fmt.Errorf("cannot convert type %T to number", raw)
	}
}

// AsArray 获取数组表示
func (a *AnswerValueAdapter) AsArray() []string {
	if a.answerValue == nil {
		return []string{}
	}

	raw := a.answerValue.Raw()
	if raw == nil {
		return []string{}
	}

	// 根据不同类型转换为数组
	switch v := raw.(type) {
	case []string:
		return v
	case string:
		// 单个字符串包装为数组
		if v == "" {
			return []string{}
		}
		return []string{v}
	default:
		// 其他类型转为字符串后包装为数组
		str := fmt.Sprintf("%v", raw)
		if str == "" {
			return []string{}
		}
		return []string{str}
	}
}

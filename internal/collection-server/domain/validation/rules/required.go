package rules

import (
	"reflect"
)

// RequiredRule 必填验证规则
type RequiredRule struct {
	*BaseRule
}

// NewRequiredRule 创建必填验证规则
func NewRequiredRule(message string) *RequiredRule {
	if message == "" {
		message = "此字段为必填项"
	}
	return &RequiredRule{
		BaseRule: NewBaseRule("required", nil, message),
	}
}

// Validate 验证值是否必填
func (r *RequiredRule) Validate(value interface{}) error {
	if value == nil {
		return NewValidationError("", r.Message, value, r.GetRuleName())
	}

	// 检查字符串是否为空
	if str, ok := value.(string); ok {
		if str == "" {
			return NewValidationError("", r.Message, value, r.GetRuleName())
		}
	}

	// 检查切片是否为空
	if reflect.TypeOf(value).Kind() == reflect.Slice {
		v := reflect.ValueOf(value)
		if v.Len() == 0 {
			return NewValidationError("", r.Message, value, r.GetRuleName())
		}
	}

	// 检查 map 是否为空
	if reflect.TypeOf(value).Kind() == reflect.Map {
		v := reflect.ValueOf(value)
		if v.Len() == 0 {
			return NewValidationError("", r.Message, value, r.GetRuleName())
		}
	}

	// 检查指针是否为空
	if reflect.TypeOf(value).Kind() == reflect.Ptr {
		v := reflect.ValueOf(value)
		if v.IsNil() {
			return NewValidationError("", r.Message, value, r.GetRuleName())
		}
	}

	return nil
}

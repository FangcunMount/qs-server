package rules

import (
	"fmt"
	"reflect"
	"strconv"
)

// MinLengthRule 最小长度验证规则
type MinLengthRule struct {
	*BaseRule
	MinLength int
}

// NewMinLengthRule 创建最小长度验证规则
func NewMinLengthRule(minLength interface{}, message string) *MinLengthRule {
	min := 0
	switch v := minLength.(type) {
	case int:
		min = v
	case int32:
		min = int(v)
	case int64:
		min = int(v)
	case float32:
		min = int(v)
	case float64:
		min = int(v)
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			min = parsed
		}
	}

	if message == "" {
		message = fmt.Sprintf("长度不能少于 %d 个字符", min)
	}

	return &MinLengthRule{
		BaseRule:  NewBaseRule("min_length", minLength, message),
		MinLength: min,
	}
}

// Validate 验证最小长度
func (r *MinLengthRule) Validate(value interface{}) error {
	if value == nil {
		return nil // 空值由 required 规则处理
	}

	length := 0
	switch v := value.(type) {
	case string:
		length = len(v)
	case []interface{}:
		length = len(v)
	case []string:
		length = len(v)
	case []int:
		length = len(v)
	case []float64:
		length = len(v)
	default:
		// 使用反射获取长度
		val := reflect.ValueOf(value)
		switch val.Kind() {
		case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
			length = val.Len()
		default:
			return NewValidationError("", "不支持长度验证的数据类型", value, r.GetRuleName())
		}
	}

	if length < r.MinLength {
		return NewValidationError("", r.Message, value, r.GetRuleName())
	}

	return nil
}

// MaxLengthRule 最大长度验证规则
type MaxLengthRule struct {
	*BaseRule
	MaxLength int
}

// NewMaxLengthRule 创建最大长度验证规则
func NewMaxLengthRule(maxLength interface{}, message string) *MaxLengthRule {
	max := 0
	switch v := maxLength.(type) {
	case int:
		max = v
	case int32:
		max = int(v)
	case int64:
		max = int(v)
	case float32:
		max = int(v)
	case float64:
		max = int(v)
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			max = parsed
		}
	}

	if message == "" {
		message = fmt.Sprintf("长度不能超过 %d 个字符", max)
	}

	return &MaxLengthRule{
		BaseRule:  NewBaseRule("max_length", maxLength, message),
		MaxLength: max,
	}
}

// Validate 验证最大长度
func (r *MaxLengthRule) Validate(value interface{}) error {
	if value == nil {
		return nil // 空值由 required 规则处理
	}

	length := 0
	switch v := value.(type) {
	case string:
		length = len(v)
	case []interface{}:
		length = len(v)
	case []string:
		length = len(v)
	case []int:
		length = len(v)
	case []float64:
		length = len(v)
	default:
		// 使用反射获取长度
		val := reflect.ValueOf(value)
		switch val.Kind() {
		case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
			length = val.Len()
		default:
			return NewValidationError("", "不支持长度验证的数据类型", value, r.GetRuleName())
		}
	}

	if length > r.MaxLength {
		return NewValidationError("", r.Message, value, r.GetRuleName())
	}

	return nil
}

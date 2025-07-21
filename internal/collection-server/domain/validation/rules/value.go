package rules

import (
	"fmt"
	"strconv"
)

// MinValueRule 最小值验证规则
type MinValueRule struct {
	*BaseRule
	MinValue float64
}

// NewMinValueRule 创建最小值验证规则
func NewMinValueRule(minValue interface{}, message string) *MinValueRule {
	min := 0.0
	switch v := minValue.(type) {
	case int:
		min = float64(v)
	case int32:
		min = float64(v)
	case int64:
		min = float64(v)
	case float32:
		min = float64(v)
	case float64:
		min = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			min = parsed
		}
	}

	if message == "" {
		message = fmt.Sprintf("值不能小于 %v", minValue)
	}

	return &MinValueRule{
		BaseRule: NewBaseRule("min_value", minValue, message),
		MinValue: min,
	}
}

// Validate 验证最小值
func (r *MinValueRule) Validate(value interface{}) error {
	if value == nil {
		return nil // 空值由 required 规则处理
	}

	val := 0.0
	switch v := value.(type) {
	case int:
		val = float64(v)
	case int32:
		val = float64(v)
	case int64:
		val = float64(v)
	case float32:
		val = float64(v)
	case float64:
		val = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err != nil {
			return NewValidationError("", "值必须是数字", value, r.GetRuleName())
		} else {
			val = parsed
		}
	default:
		return NewValidationError("", "不支持数值验证的数据类型", value, r.GetRuleName())
	}

	if val < r.MinValue {
		return NewValidationError("", r.Message, value, r.GetRuleName())
	}

	return nil
}

// MaxValueRule 最大值验证规则
type MaxValueRule struct {
	*BaseRule
	MaxValue float64
}

// NewMaxValueRule 创建最大值验证规则
func NewMaxValueRule(maxValue interface{}, message string) *MaxValueRule {
	max := 0.0
	switch v := maxValue.(type) {
	case int:
		max = float64(v)
	case int32:
		max = float64(v)
	case int64:
		max = float64(v)
	case float32:
		max = float64(v)
	case float64:
		max = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			max = parsed
		}
	}

	if message == "" {
		message = fmt.Sprintf("值不能大于 %v", maxValue)
	}

	return &MaxValueRule{
		BaseRule: NewBaseRule("max_value", maxValue, message),
		MaxValue: max,
	}
}

// Validate 验证最大值
func (r *MaxValueRule) Validate(value interface{}) error {
	if value == nil {
		return nil // 空值由 required 规则处理
	}

	val := 0.0
	switch v := value.(type) {
	case int:
		val = float64(v)
	case int32:
		val = float64(v)
	case int64:
		val = float64(v)
	case float32:
		val = float64(v)
	case float64:
		val = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err != nil {
			return NewValidationError("", "值必须是数字", value, r.GetRuleName())
		} else {
			val = parsed
		}
	default:
		return NewValidationError("", "不支持数值验证的数据类型", value, r.GetRuleName())
	}

	if val > r.MaxValue {
		return NewValidationError("", r.Message, value, r.GetRuleName())
	}

	return nil
}

// RangeRule 范围验证规则
type RangeRule struct {
	*BaseRule
	MinValue float64
	MaxValue float64
}

// NewRangeRule 创建范围验证规则
func NewRangeRule(minValue, maxValue interface{}, message string) *RangeRule {
	min := 0.0
	max := 0.0

	// 解析最小值
	switch v := minValue.(type) {
	case int:
		min = float64(v)
	case int32:
		min = float64(v)
	case int64:
		min = float64(v)
	case float32:
		min = float64(v)
	case float64:
		min = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			min = parsed
		}
	}

	// 解析最大值
	switch v := maxValue.(type) {
	case int:
		max = float64(v)
	case int32:
		max = float64(v)
	case int64:
		max = float64(v)
	case float32:
		max = float64(v)
	case float64:
		max = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			max = parsed
		}
	}

	if message == "" {
		message = fmt.Sprintf("值必须在 %v 到 %v 之间", minValue, maxValue)
	}

	return &RangeRule{
		BaseRule: NewBaseRule("range", fmt.Sprintf("%v-%v", minValue, maxValue), message),
		MinValue: min,
		MaxValue: max,
	}
}

// Validate 验证范围
func (r *RangeRule) Validate(value interface{}) error {
	if value == nil {
		return nil // 空值由 required 规则处理
	}

	val := 0.0
	switch v := value.(type) {
	case int:
		val = float64(v)
	case int32:
		val = float64(v)
	case int64:
		val = float64(v)
	case float32:
		val = float64(v)
	case float64:
		val = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err != nil {
			return NewValidationError("", "值必须是数字", value, r.GetRuleName())
		} else {
			val = parsed
		}
	default:
		return NewValidationError("", "不支持数值验证的数据类型", value, r.GetRuleName())
	}

	if val < r.MinValue || val > r.MaxValue {
		return NewValidationError("", r.Message, value, r.GetRuleName())
	}

	return nil
}

package validation

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// RequiredStrategy 必填验证策略
type RequiredStrategy struct{}

func (s *RequiredStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return NewValidationError("", rule.Message, value)
	}

	// 检查字符串类型
	if str, ok := value.(string); ok {
		if strings.TrimSpace(str) == "" {
			return NewValidationError("", rule.Message, value)
		}
		return nil
	}

	// 检查选项值类型
	if optionValue, ok := value.(map[string]interface{}); ok {
		if code, exists := optionValue["code"]; exists {
			if codeStr, ok := code.(string); ok && strings.TrimSpace(codeStr) == "" {
				return NewValidationError("", rule.Message, value)
			}
		}
		return nil
	}

	// 检查切片类型
	if reflect.TypeOf(value).Kind() == reflect.Slice {
		v := reflect.ValueOf(value)
		if v.Len() == 0 {
			return NewValidationError("", rule.Message, value)
		}
		return nil
	}

	// 检查指针类型
	if reflect.TypeOf(value).Kind() == reflect.Ptr {
		if reflect.ValueOf(value).IsNil() {
			return NewValidationError("", rule.Message, value)
		}
		return nil
	}

	return nil
}

func (s *RequiredStrategy) GetStrategyName() string {
	return "required"
}

// OptionCodeStrategy 选项代码验证策略
type OptionCodeStrategy struct{}

func (s *OptionCodeStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	// 获取允许的选项代码列表
	allowedCodes, ok := rule.Value.([]string)
	if !ok {
		return fmt.Errorf("选项验证规则的值必须是字符串数组")
	}

	// 提取选项代码
	var code string
	switch v := value.(type) {
	case string:
		code = v
	case map[string]interface{}:
		if codeVal, exists := v["code"]; exists {
			if codeStr, ok := codeVal.(string); ok {
				code = codeStr
			}
		}
	default:
		// 尝试转换为字符串
		code = fmt.Sprintf("%v", v)
	}

	// 检查代码是否在允许列表中
	for _, allowedCode := range allowedCodes {
		if code == allowedCode {
			return nil
		}
	}

	return NewValidationError("", rule.Message, value)
}

func (s *OptionCodeStrategy) GetStrategyName() string {
	return "option_code"
}

// MaxValueStrategy 最大值验证策略
type MaxValueStrategy struct{}

func (s *MaxValueStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	maxValue, ok := rule.Value.(float64)
	if !ok {
		return fmt.Errorf("最大值规则的值必须是数字类型")
	}

	// 转换为 float64 进行比较
	var currentValue float64
	switch v := value.(type) {
	case int:
		currentValue = float64(v)
	case int8:
		currentValue = float64(v)
	case int16:
		currentValue = float64(v)
	case int32:
		currentValue = float64(v)
	case int64:
		currentValue = float64(v)
	case uint:
		currentValue = float64(v)
	case uint8:
		currentValue = float64(v)
	case uint16:
		currentValue = float64(v)
	case uint32:
		currentValue = float64(v)
	case uint64:
		currentValue = float64(v)
	case float32:
		currentValue = float64(v)
	case float64:
		currentValue = v
	case string:
		// 尝试将字符串转换为数字
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			currentValue = parsed
		} else {
			return NewValidationError("", "答案必须是数字类型", value)
		}
	default:
		return NewValidationError("", "答案必须是数字类型", value)
	}

	if currentValue > maxValue {
		return NewValidationError("", rule.Message, value)
	}

	return nil
}

func (s *MaxValueStrategy) GetStrategyName() string {
	return "max_value"
}

// MinValueStrategy 最小值验证策略
type MinValueStrategy struct{}

func (s *MinValueStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	minValue, ok := rule.Value.(float64)
	if !ok {
		return fmt.Errorf("最小值规则的值必须是数字类型")
	}

	// 转换为 float64 进行比较
	var currentValue float64
	switch v := value.(type) {
	case int:
		currentValue = float64(v)
	case int8:
		currentValue = float64(v)
	case int16:
		currentValue = float64(v)
	case int32:
		currentValue = float64(v)
	case int64:
		currentValue = float64(v)
	case uint:
		currentValue = float64(v)
	case uint8:
		currentValue = float64(v)
	case uint16:
		currentValue = float64(v)
	case uint32:
		currentValue = float64(v)
	case uint64:
		currentValue = float64(v)
	case float32:
		currentValue = float64(v)
	case float64:
		currentValue = v
	case string:
		// 尝试将字符串转换为数字
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			currentValue = parsed
		} else {
			return NewValidationError("", "答案必须是数字类型", value)
		}
	default:
		return NewValidationError("", "答案必须是数字类型", value)
	}

	if currentValue < minValue {
		return NewValidationError("", rule.Message, value)
	}

	return nil
}

func (s *MinValueStrategy) GetStrategyName() string {
	return "min_value"
}

// MaxLengthStrategy 最大长度验证策略
type MaxLengthStrategy struct{}

func (s *MaxLengthStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	maxLength, ok := rule.Value.(int)
	if !ok {
		return fmt.Errorf("最大长度规则的值必须是整数类型")
	}

	var currentLength int
	switch v := value.(type) {
	case string:
		currentLength = len(v)
	case []byte:
		currentLength = len(v)
	case []rune:
		currentLength = len(v)
	default:
		// 对于其他类型，尝试转换为字符串
		if reflect.TypeOf(value).Kind() == reflect.Slice {
			currentLength = reflect.ValueOf(value).Len()
		} else {
			str := fmt.Sprintf("%v", value)
			currentLength = len(str)
		}
	}

	if currentLength > maxLength {
		return NewValidationError("", rule.Message, value)
	}

	return nil
}

func (s *MaxLengthStrategy) GetStrategyName() string {
	return "max_length"
}

// MinLengthStrategy 最小长度验证策略
type MinLengthStrategy struct{}

func (s *MinLengthStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	minLength, ok := rule.Value.(int)
	if !ok {
		return fmt.Errorf("最小长度规则的值必须是整数类型")
	}

	var currentLength int
	switch v := value.(type) {
	case string:
		currentLength = len(v)
	case []byte:
		currentLength = len(v)
	case []rune:
		currentLength = len(v)
	default:
		// 对于其他类型，尝试转换为字符串
		if reflect.TypeOf(value).Kind() == reflect.Slice {
			currentLength = reflect.ValueOf(value).Len()
		} else {
			str := fmt.Sprintf("%v", value)
			currentLength = len(str)
		}
	}

	if currentLength < minLength {
		return NewValidationError("", rule.Message, value)
	}

	return nil
}

func (s *MinLengthStrategy) GetStrategyName() string {
	return "min_length"
}

// PatternStrategy 正则表达式验证策略
type PatternStrategy struct{}

func (s *PatternStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	pattern, ok := rule.Value.(string)
	if !ok {
		return fmt.Errorf("正则表达式规则的值必须是字符串类型")
	}

	// 将值转换为字符串
	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	default:
		strValue = fmt.Sprintf("%v", v)
	}

	// 编译正则表达式
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("无效的正则表达式: %v", err)
	}

	if !regex.MatchString(strValue) {
		return NewValidationError("", rule.Message, value)
	}

	return nil
}

func (s *PatternStrategy) GetStrategyName() string {
	return "pattern"
}

// EmailStrategy 邮箱验证策略
type EmailStrategy struct{}

func (s *EmailStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	strValue, ok := value.(string)
	if !ok {
		return NewValidationError("", "邮箱答案必须是字符串类型", value)
	}

	// 简单的邮箱格式验证
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(strValue) {
		return NewValidationError("", rule.Message, value)
	}

	return nil
}

func (s *EmailStrategy) GetStrategyName() string {
	return "email"
}

// PhoneStrategy 手机号验证策略
type PhoneStrategy struct{}

func (s *PhoneStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	strValue, ok := value.(string)
	if !ok {
		return NewValidationError("", "手机号答案必须是字符串类型", value)
	}

	// 简单的手机号格式验证（中国大陆）
	phoneRegex := regexp.MustCompile(`^1[3-9]\d{9}$`)
	if !phoneRegex.MatchString(strValue) {
		return NewValidationError("", rule.Message, value)
	}

	return nil
}

func (s *PhoneStrategy) GetStrategyName() string {
	return "phone"
}

// RangeStrategy 数值范围验证策略
type RangeStrategy struct{}

func (s *RangeStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	// 获取范围参数
	minValue, maxValue := s.extractRangeValues(rule)
	if minValue == nil || maxValue == nil {
		return fmt.Errorf("范围验证规则需要指定最小值和最大值")
	}

	// 转换为 float64 进行比较
	currentValue, err := s.convertToFloat64(value)
	if err != nil {
		return NewValidationError("", "答案必须是数字类型", value)
	}

	if currentValue < minValue.(float64) || currentValue > maxValue.(float64) {
		return NewValidationError("", rule.Message, value)
	}

	return nil
}

func (s *RangeStrategy) extractRangeValues(rule *ValidationRule) (min, max interface{}) {
	if rule.Params != nil {
		min = rule.Params["min"]
		max = rule.Params["max"]
	}
	if min == nil || max == nil {
		// 尝试从 Value 中提取范围（假设是数组）
		if arr, ok := rule.Value.([]interface{}); ok && len(arr) == 2 {
			min = arr[0]
			max = arr[1]
		}
	}
	return min, max
}

func (s *RangeStrategy) convertToFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("unsupported type")
	}
}

func (s *RangeStrategy) GetStrategyName() string {
	return "range"
}

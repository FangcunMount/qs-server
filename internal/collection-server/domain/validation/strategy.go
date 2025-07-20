package validation

import (
	"fmt"
)

// ValidationStrategy 验证策略接口
type ValidationStrategy interface {
	Validate(value interface{}, rule *ValidationRule) error
	GetStrategyName() string
}

// ValidationRule 验证规则
type ValidationRule struct {
	Strategy string                 `json:"strategy"` // 策略名称
	Value    interface{}            `json:"value"`    // 规则值
	Message  string                 `json:"message"`  // 错误消息
	Params   map[string]interface{} `json:"params"`   // 额外参数
}

// NewValidationRule 创建验证规则
func NewValidationRule(strategy string, value interface{}, message string) *ValidationRule {
	return &ValidationRule{
		Strategy: strategy,
		Value:    value,
		Message:  message,
		Params:   make(map[string]interface{}),
	}
}

// WithParams 添加额外参数
func (r *ValidationRule) WithParams(params map[string]interface{}) *ValidationRule {
	r.Params = params
	return r
}

// AddParam 添加单个参数
func (r *ValidationRule) AddParam(key string, value interface{}) *ValidationRule {
	if r.Params == nil {
		r.Params = make(map[string]interface{})
	}
	r.Params[key] = value
	return r
}

// ValidationError 验证错误
type ValidationError struct {
	Field   string      `json:"field"`
	Message string      `json:"message"`
	Value   interface{} `json:"value"`
}

// Error 实现 error 接口
func (e *ValidationError) Error() string {
	return fmt.Sprintf("答案验证失败: %s (值: %v)", e.Message, e.Value)
}

// NewValidationError 创建验证错误
func NewValidationError(field, message string, value interface{}) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	}
}

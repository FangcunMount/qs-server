package rules

import (
	"fmt"
)

// Rule 验证规则接口
type Rule interface {
	Validate(value interface{}) error
	GetRuleName() string
}

// BaseRule 基础验证规则
type BaseRule struct {
	Name    string                 `json:"name"`
	Value   interface{}            `json:"value"`
	Message string                 `json:"message"`
	Params  map[string]interface{} `json:"params"`
}

// NewBaseRule 创建基础验证规则
func NewBaseRule(name string, value interface{}, message string) *BaseRule {
	return &BaseRule{
		Name:    name,
		Value:   value,
		Message: message,
		Params:  make(map[string]interface{}),
	}
}

// GetRuleName 获取规则名称
func (r *BaseRule) GetRuleName() string {
	return r.Name
}

// WithParams 添加参数
func (r *BaseRule) WithParams(params map[string]interface{}) *BaseRule {
	r.Params = params
	return r
}

// AddParam 添加单个参数
func (r *BaseRule) AddParam(key string, value interface{}) *BaseRule {
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
	Rule    string      `json:"rule"`
}

// Error 实现 error 接口
func (e *ValidationError) Error() string {
	return fmt.Sprintf("验证失败: %s (值: %v, 规则: %s)", e.Message, e.Value, e.Rule)
}

// NewValidationError 创建验证错误
func NewValidationError(field, message string, value interface{}, rule string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
		Rule:    rule,
	}
}

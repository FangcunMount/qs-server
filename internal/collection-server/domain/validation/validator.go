package validation

import (
	"fmt"
	"reflect"
	"sync"
)

// Validator 验证器
type Validator struct {
	strategies map[string]ValidationStrategy
	mu         sync.RWMutex
}

// NewValidator 创建验证器
func NewValidator() *Validator {
	validator := &Validator{
		strategies: make(map[string]ValidationStrategy),
	}

	// 注册默认策略
	validator.RegisterDefaultStrategies()

	return validator
}

// RegisterDefaultStrategies 注册默认验证策略
func (v *Validator) RegisterDefaultStrategies() {
	v.RegisterStrategy(&RequiredStrategy{})
	v.RegisterStrategy(&MaxValueStrategy{})
	v.RegisterStrategy(&MinValueStrategy{})
	v.RegisterStrategy(&MaxLengthStrategy{})
	v.RegisterStrategy(&MinLengthStrategy{})
	v.RegisterStrategy(&PatternStrategy{})
	v.RegisterStrategy(&EmailStrategy{})
}

// RegisterStrategy 注册验证策略
func (v *Validator) RegisterStrategy(strategy ValidationStrategy) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.strategies[strategy.GetStrategyName()] = strategy
}

// GetStrategy 获取验证策略
func (v *Validator) GetStrategy(name string) (ValidationStrategy, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	strategy, exists := v.strategies[name]
	if !exists {
		return nil, fmt.Errorf("验证策略 '%s' 不存在", name)
	}

	return strategy, nil
}

// Validate 验证单个值
func (v *Validator) Validate(value interface{}, rule *ValidationRule) error {
	if rule == nil {
		return nil
	}

	strategy, err := v.GetStrategy(rule.Strategy)
	if err != nil {
		return err
	}

	return strategy.Validate(value, rule)
}

// ValidateField 验证字段
func (v *Validator) ValidateField(fieldName string, value interface{}, rule *ValidationRule) error {
	if rule == nil {
		return nil
	}

	err := v.Validate(value, rule)
	if err != nil {
		// 如果是 ValidationError，设置字段名
		if validationErr, ok := err.(*ValidationError); ok {
			validationErr.Field = fieldName
		}
	}

	return err
}

// ValidateMultiple 验证多个规则
func (v *Validator) ValidateMultiple(value interface{}, rules []*ValidationRule) []error {
	var errors []error

	for _, rule := range rules {
		if err := v.Validate(value, rule); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

// ValidateMultipleFields 验证多个字段
func (v *Validator) ValidateMultipleFields(fields map[string]interface{}, rules map[string][]*ValidationRule) map[string][]error {
	errors := make(map[string][]error)

	for fieldName, value := range fields {
		if fieldRules, exists := rules[fieldName]; exists {
			fieldErrors := v.ValidateMultiple(value, fieldRules)
			if len(fieldErrors) > 0 {
				// 为每个错误设置字段名
				for _, err := range fieldErrors {
					if validationErr, ok := err.(*ValidationError); ok {
						validationErr.Field = fieldName
					}
				}
				errors[fieldName] = fieldErrors
			}
		}
	}

	return errors
}

// ValidateStruct 验证结构体
func (v *Validator) ValidateStruct(data interface{}, rules map[string][]*ValidationRule) map[string][]error {
	// 使用反射获取结构体字段
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return map[string][]error{
			"": {fmt.Errorf("数据必须是结构体类型")},
		}
	}

	errors := make(map[string][]error)
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// 获取字段名（支持 json 标签）
		fieldName := fieldType.Name
		if jsonTag := fieldType.Tag.Get("json"); jsonTag != "" {
			if jsonTag != "-" {
				fieldName = jsonTag
			}
		}

		// 检查是否有验证规则
		if fieldRules, exists := rules[fieldName]; exists {
			fieldValue := field.Interface()
			fieldErrors := v.ValidateMultiple(fieldValue, fieldRules)

			if len(fieldErrors) > 0 {
				// 为每个错误设置字段名
				for _, err := range fieldErrors {
					if validationErr, ok := err.(*ValidationError); ok {
						validationErr.Field = fieldName
					}
				}
				errors[fieldName] = fieldErrors
			}
		}
	}

	return errors
}

// HasErrors 检查是否有验证错误
func (v *Validator) HasErrors(errors map[string][]error) bool {
	for _, fieldErrors := range errors {
		if len(fieldErrors) > 0 {
			return true
		}
	}
	return false
}

// GetFirstError 获取第一个验证错误
func (v *Validator) GetFirstError(errors map[string][]error) error {
	for _, fieldErrors := range errors {
		if len(fieldErrors) > 0 {
			return fieldErrors[0]
		}
	}
	return nil
}

// GetAllErrors 获取所有验证错误
func (v *Validator) GetAllErrors(errors map[string][]error) []error {
	var allErrors []error
	for _, fieldErrors := range errors {
		allErrors = append(allErrors, fieldErrors...)
	}
	return allErrors
}

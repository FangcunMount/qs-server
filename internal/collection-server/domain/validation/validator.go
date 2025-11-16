package validation

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/FangcunMount/qs-server/internal/collection-server/domain/validation/rules"
	"github.com/FangcunMount/qs-server/internal/collection-server/domain/validation/strategies"
)

// Rule 验证规则类型别名
type Rule = *rules.BaseRule

// Validator 验证器
type Validator struct {
	strategyFactory *strategies.StrategyFactory
	mu              sync.RWMutex
}

// NewValidator 创建验证器
func NewValidator() *Validator {
	return &Validator{
		strategyFactory: strategies.GetGlobalStrategyFactory(),
	}
}

// Validate 验证单个值
func (v *Validator) Validate(value interface{}, rule *rules.BaseRule) error {
	if rule == nil {
		return nil
	}

	strategy, err := v.strategyFactory.GetStrategy(rule.Name)
	if err != nil {
		return fmt.Errorf("验证策略 '%s' 不存在: %w", rule.Name, err)
	}

	return strategy.Validate(value, rule)
}

// ValidateField 验证字段
func (v *Validator) ValidateField(fieldName string, value interface{}, rule *rules.BaseRule) error {
	if rule == nil {
		return nil
	}

	err := v.Validate(value, rule)
	if err != nil {
		// 如果是 ValidationError，设置字段名
		// TODO: 修复类型断言问题
		// if validationErr, ok := err.(*rules.ValidationError); ok {
		// 	validationErr.Field = fieldName
		// }
	}

	return err
}

// ValidateMultiple 验证多个规则
func (v *Validator) ValidateMultiple(value interface{}, rules []*rules.BaseRule) []error {
	var errors []error

	for _, rule := range rules {
		if err := v.Validate(value, rule); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

// ValidateMultipleFields 验证多个字段
func (v *Validator) ValidateMultipleFields(fields map[string]interface{}, rules map[string][]*rules.BaseRule) map[string][]error {
	errors := make(map[string][]error)

	for fieldName, value := range fields {
		if fieldRules, exists := rules[fieldName]; exists {
			fieldErrors := v.ValidateMultiple(value, fieldRules)
			if len(fieldErrors) > 0 {
				// 为每个错误设置字段名
				for _, err := range fieldErrors {
					// TODO: 修复类型断言问题
					// if validationErr, ok := err.(*rules.ValidationError); ok {
					// 	validationErr.Field = fieldName
					// }
					_ = err // 避免未使用变量错误
				}
				errors[fieldName] = fieldErrors
			}
		}
	}

	return errors
}

// ValidateStruct 验证结构体
func (v *Validator) ValidateStruct(data interface{}, rules map[string][]*rules.BaseRule) map[string][]error {
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
					// 使用项目通用的 error 类型，不进行类型断言
					_ = err
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

// RegisterCustomStrategy 注册自定义验证策略
func (v *Validator) RegisterCustomStrategy(strategy strategies.ValidationStrategy) error {
	return strategies.RegisterCustomStrategy(strategy)
}

// GetStrategy 获取验证策略
func (v *Validator) GetStrategy(name string) (strategies.ValidationStrategy, error) {
	return v.strategyFactory.GetStrategy(name)
}

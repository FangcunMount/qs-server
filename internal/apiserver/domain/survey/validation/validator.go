package validation

// Validator 校验器接口
//
// 设计原则：validation 领域只关心"规则 + 值 → 结果"
// 不依赖问卷、答卷等业务对象
type Validator interface {
	// ValidateValue 校验单个值
	// value: 被校验的值
	// rules: 应用的校验规则列表
	ValidateValue(value ValidatableValue, rules []ValidationRule) *ValidationResult
}

// DefaultValidator 默认校验器
type DefaultValidator struct{}

// NewDefaultValidator 创建默认校验器
func NewDefaultValidator() *DefaultValidator {
	return &DefaultValidator{}
}

// ValidateValue 校验单个值
func (v *DefaultValidator) ValidateValue(
	value ValidatableValue,
	rules []ValidationRule,
) *ValidationResult {
	result := NewValidationResult()

	// 遍历所有校验规则
	for _, rule := range rules {
		// 获取对应的校验策略
		strategy := GetStrategy(rule.GetRuleType())
		if strategy == nil {
			// 没有对应的策略，跳过
			continue
		}

		// 执行校验
		if err := strategy.Validate(value, rule); err != nil {
			result.AddError(ValidationError{
				ruleType: string(rule.GetRuleType()),
				message:  err.Error(),
			})
		}
	}

	return result
}

// ValidationResult 校验结果
type ValidationResult struct {
	valid  bool
	errors []ValidationError
}

// NewValidationResult 创建校验结果
func NewValidationResult() *ValidationResult {
	return &ValidationResult{
		valid:  true,
		errors: []ValidationError{},
	}
}

// IsValid 是否校验通过
func (r *ValidationResult) IsValid() bool {
	return r.valid
}

// GetErrors 获取校验错误列表
func (r *ValidationResult) GetErrors() []ValidationError {
	return r.errors
}

// AddError 添加校验错误
func (r *ValidationResult) AddError(err ValidationError) {
	r.valid = false
	r.errors = append(r.errors, err)
}

// ValidationError 校验错误
type ValidationError struct {
	ruleType string // 校验规则类型
	message  string // 错误信息
}

// GetRuleType 获取校验规则类型
func (e ValidationError) GetRuleType() string {
	return e.ruleType
}

// GetMessage 获取错误信息
func (e ValidationError) GetMessage() string {
	return e.message
}

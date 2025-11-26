package validation

// RuleType 校验规则类型
type RuleType string

const (
	// RuleTypeRequired 必填规则
	RuleTypeRequired RuleType = "required"

	// RuleTypeMinLength 最小长度规则
	RuleTypeMinLength RuleType = "min_length"

	// RuleTypeMaxLength 最大长度规则
	RuleTypeMaxLength RuleType = "max_length"

	// RuleTypeMinValue 最小值规则
	RuleTypeMinValue RuleType = "min_value"

	// RuleTypeMaxValue 最大值规则
	RuleTypeMaxValue RuleType = "max_value"

	// RuleTypeMinSelections 最少选项数规则
	RuleTypeMinSelections RuleType = "min_selections"

	// RuleTypeMaxSelections 最多选项数规则
	RuleTypeMaxSelections RuleType = "max_selections"

	// RuleTypePattern 正则表达式规则
	RuleTypePattern RuleType = "pattern"
)

// ValidationRule 校验规则（值对象）
// 定义问题的校验配置
type ValidationRule struct {
	ruleType    RuleType
	targetValue string
}

// NewValidationRule 创建校验规则
func NewValidationRule(ruleType RuleType, targetValue string) ValidationRule {
	return ValidationRule{
		ruleType:    ruleType,
		targetValue: targetValue,
	}
}

// GetRuleType 获取规则类型
func (r ValidationRule) GetRuleType() RuleType {
	return r.ruleType
}

// GetTargetValue 获取目标值
func (r ValidationRule) GetTargetValue() string {
	return r.targetValue
}

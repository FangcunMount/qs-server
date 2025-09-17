package validation

type RuleType string

const (
	RuleTypeRequired      RuleType = "required"
	RuleTypeMinLength     RuleType = "min_length"
	RuleTypeMaxLength     RuleType = "max_length"
	RuleTypeMinValue      RuleType = "min_value"
	RuleTypeMaxValue      RuleType = "max_value"
	RuleTypeMinSelections RuleType = "min_selections"
	RuleTypeMaxSelections RuleType = "max_selections"
)

// ValidationRule 校验规则接口
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
func (r *ValidationRule) GetRuleType() RuleType {
	return r.ruleType
}

// GetTargetValue 获取目标值
func (r *ValidationRule) GetTargetValue() string {
	return r.targetValue
}

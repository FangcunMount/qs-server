package vo

// RuleType 规则类型
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

// ValidationAbility 校验能力
type ValidationAbility struct {
	validationRules []ValidationRule
}

// GetValidationRules 获取校验规则
func (v *ValidationAbility) GetValidationRules() []ValidationRule {
	return v.validationRules
}

// AddValidationRule 添加校验规则
func (v *ValidationAbility) AddValidationRule(rule ValidationRule) {
	// 如果校验规则已存在，则不添加
	for _, r := range v.validationRules {
		if r.GetRuleType() == rule.GetRuleType() {
			return
		}
	}

	// 如果校验规则不存在，则添加
	v.validationRules = append(v.validationRules, rule)
}

// ClearValidationRules 清空校验规则
func (v *ValidationAbility) ClearValidationRules() {
	v.validationRules = []ValidationRule{}
}

// ValidationRule 校验规则接口
type ValidationRule struct {
	ruleType    RuleType
	ruleName    string
	targetValue string
}

// GetRuleType 获取规则类型
func (r *ValidationRule) GetRuleType() RuleType {
	return r.ruleType
}

// GetRuleName 获取规则名称
func (r *ValidationRule) GetRuleName() string {
	return r.ruleName
}

// GetTargetValue 获取目标值
func (r *ValidationRule) GetTargetValue() string {
	return r.targetValue
}

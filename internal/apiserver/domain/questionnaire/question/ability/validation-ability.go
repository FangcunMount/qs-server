package ability

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

// SetValidationRules 设置校验规则
func (v *ValidationAbility) SetValidationRules(rules []ValidationRule) {
	v.validationRules = rules
}

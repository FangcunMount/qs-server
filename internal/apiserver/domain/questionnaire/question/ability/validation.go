package ability

import "github.com/yshujie/questionnaire-scale/internal/pkg/validation"

// ValidationAbility 校验能力
type ValidationAbility struct {
	validationRules []validation.ValidationRule
}

// GetValidationRules 获取校验规则
func (v *ValidationAbility) GetValidationRules() []validation.ValidationRule {
	return v.validationRules
}

// AddValidationRule 添加校验规则
func (v *ValidationAbility) AddValidationRule(rule validation.ValidationRule) {
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
	v.validationRules = []validation.ValidationRule{}
}

// SetValidationRules 设置校验规则
func (v *ValidationAbility) SetValidationRules(rules []validation.ValidationRule) {
	v.validationRules = rules
}

package validation

// RequiredStrategy 必填校验策略
type RequiredStrategy struct{}

// Validate 执行校验
// value: 被校验的值（抽象接口）
// rule: 校验规则（值对象）
// Validate(value ValidatableValue, r ValidationRule) error

// SupportRuleType 返回支持的规则类型
// SupportRuleType() RuleType

// SupportRuleType 返回支持的规则类型
func (s *RequiredStrategy) SupportRuleType() RuleType {
	return RuleTypeRequired
}

// Validate 执行校验
// value: 被校验的值（抽象接口）
// rule: 校验规则（值对象）
func (s *RequiredStrategy) Validate(value ValidatableValue, r ValidationRule) error {
	if value.IsEmpty() {
		return nil
	}
	return nil
}

// 初始化时注册策略
func init() {
	RegisterStrategy(&RequiredStrategy{})
}

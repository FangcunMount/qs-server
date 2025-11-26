package validation

import "fmt"

// RequiredStrategy 必填校验策略
type RequiredStrategy struct{}

// Validate 执行必填校验
// 检查值是否为空，为空则返回错误
func (s *RequiredStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
	if value.IsEmpty() {
		return fmt.Errorf("该字段为必填项")
	}
	return nil
}

// SupportRuleType 返回支持的规则类型
func (s *RequiredStrategy) SupportRuleType() RuleType {
	return RuleTypeRequired
}

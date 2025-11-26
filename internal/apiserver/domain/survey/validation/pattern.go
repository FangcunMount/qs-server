package validation

import (
	"fmt"
	"regexp"
)

// PatternStrategy 正则表达式校验策略
type PatternStrategy struct{}

// Validate 执行正则表达式校验
// 检查值是否匹配指定的正则表达式模式
func (s *PatternStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
	// 空值跳过
	if value.IsEmpty() {
		return nil
	}

	// 获取正则表达式模式
	pattern := rule.GetTargetValue()
	if pattern == "" {
		return fmt.Errorf("pattern rule requires a non-empty pattern")
	}

	// 编译正则表达式
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %v", err)
	}

	// 获取字符串值
	str := value.AsString()

	// 执行匹配
	if !regex.MatchString(str) {
		return fmt.Errorf("输入格式不正确")
	}

	return nil
}

// SupportRuleType 返回支持的规则类型
func (s *PatternStrategy) SupportRuleType() RuleType {
	return RuleTypePattern
}

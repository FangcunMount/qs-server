package validation

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// MinLengthStrategy 最小长度校验策略
type MinLengthStrategy struct{}

// Validate 执行最小长度校验
// 检查字符串的字符数（非字节数）是否达到最小长度要求
func (s *MinLengthStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
	// 空值跳过
	if value.IsEmpty() {
		return nil
	}

	// 获取目标长度
	minLength, err := strconv.Atoi(rule.GetTargetValue())
	if err != nil {
		return fmt.Errorf("invalid min_length rule value: %s", rule.GetTargetValue())
	}

	// 获取字符串值
	str := value.AsString()

	// 计算字符数（使用 UTF-8 编码）
	actualLength := utf8.RuneCountInString(str)

	if actualLength < minLength {
		return fmt.Errorf("字符数不得少于 %d 个", minLength)
	}

	return nil
}

// SupportRuleType 返回支持的规则类型
func (s *MinLengthStrategy) SupportRuleType() RuleType {
	return RuleTypeMinLength
}

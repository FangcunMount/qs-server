package validation

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// MaxLengthStrategy 最大长度校验策略
type MaxLengthStrategy struct{}

// Validate 执行最大长度校验
// 检查字符串的字符数（非字节数）是否超过最大长度限制
func (s *MaxLengthStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
	// 空值跳过
	if value.IsEmpty() {
		return nil
	}

	// 获取目标长度
	maxLength, err := strconv.Atoi(rule.GetTargetValue())
	if err != nil {
		return fmt.Errorf("invalid max_length rule value: %s", rule.GetTargetValue())
	}

	// 获取字符串值
	str := value.AsString()

	// 计算字符数（使用 UTF-8 编码）
	actualLength := utf8.RuneCountInString(str)

	if actualLength > maxLength {
		return fmt.Errorf("字符数不得超过 %d 个", maxLength)
	}

	return nil
}

// SupportRuleType 返回支持的规则类型
func (s *MaxLengthStrategy) SupportRuleType() RuleType {
	return RuleTypeMaxLength
}

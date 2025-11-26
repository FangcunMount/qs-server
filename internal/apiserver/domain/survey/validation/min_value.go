package validation

import (
	"fmt"
	"strconv"
)

// MinValueStrategy 最小值校验策略
type MinValueStrategy struct{}

// Validate 执行最小值校验
// 检查数值是否达到最小值要求
func (s *MinValueStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
	// 空值跳过
	if value.IsEmpty() {
		return nil
	}

	// 获取目标最小值
	minValue, err := strconv.ParseFloat(rule.GetTargetValue(), 64)
	if err != nil {
		return fmt.Errorf("invalid min_value rule value: %s", rule.GetTargetValue())
	}

	// 获取数值
	actualValue, err := value.AsNumber()
	if err != nil {
		return fmt.Errorf("无法将值转换为数字: %v", err)
	}

	if actualValue < minValue {
		return fmt.Errorf("值不得小于 %v", minValue)
	}

	return nil
}

// SupportRuleType 返回支持的规则类型
func (s *MinValueStrategy) SupportRuleType() RuleType {
	return RuleTypeMinValue
}

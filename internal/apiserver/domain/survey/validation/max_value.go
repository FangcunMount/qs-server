package validation

import (
	"fmt"
	"strconv"
)

// MaxValueStrategy 最大值校验策略
type MaxValueStrategy struct{}

// Validate 执行最大值校验
// 检查数值是否超过最大值限制
func (s *MaxValueStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
	// 空值跳过
	if value.IsEmpty() {
		return nil
	}

	// 获取目标最大值
	maxValue, err := strconv.ParseFloat(rule.GetTargetValue(), 64)
	if err != nil {
		return fmt.Errorf("invalid max_value rule value: %s", rule.GetTargetValue())
	}

	// 获取数值
	actualValue, err := value.AsNumber()
	if err != nil {
		return fmt.Errorf("无法将值转换为数字: %v", err)
	}

	if actualValue > maxValue {
		return fmt.Errorf("值不得大于 %v", maxValue)
	}

	return nil
}

// SupportRuleType 返回支持的规则类型
func (s *MaxValueStrategy) SupportRuleType() RuleType {
	return RuleTypeMaxValue
}

package validation

import (
	"fmt"
	"strconv"
)

// MinSelectionsStrategy 最少选项数校验策略
type MinSelectionsStrategy struct{}

// Validate 执行最少选项数校验
// 检查多选题答案是否选择了足够数量的选项
func (s *MinSelectionsStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
	// 空值跳过
	if value.IsEmpty() {
		return nil
	}

	// 获取目标最少选项数
	minSelections, err := strconv.Atoi(rule.GetTargetValue())
	if err != nil {
		return fmt.Errorf("invalid min_selections rule value: %s", rule.GetTargetValue())
	}

	// 获取选项数组
	selections := value.AsArray()
	actualCount := len(selections)

	if actualCount < minSelections {
		return fmt.Errorf("至少需要选择 %d 项", minSelections)
	}

	return nil
}

// SupportRuleType 返回支持的规则类型
func (s *MinSelectionsStrategy) SupportRuleType() RuleType {
	return RuleTypeMinSelections
}

// MaxSelectionsStrategy 最多选项数校验策略
type MaxSelectionsStrategy struct{}

// Validate 执行最多选项数校验
// 检查多选题答案是否超过选项数量限制
func (s *MaxSelectionsStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
	// 空值跳过
	if value.IsEmpty() {
		return nil
	}

	// 获取目标最多选项数
	maxSelections, err := strconv.Atoi(rule.GetTargetValue())
	if err != nil {
		return fmt.Errorf("invalid max_selections rule value: %s", rule.GetTargetValue())
	}

	// 获取选项数组
	selections := value.AsArray()
	actualCount := len(selections)

	if actualCount > maxSelections {
		return fmt.Errorf("最多只能选择 %d 项", maxSelections)
	}

	return nil
}

// SupportRuleType 返回支持的规则类型
func (s *MaxSelectionsStrategy) SupportRuleType() RuleType {
	return RuleTypeMaxSelections
}

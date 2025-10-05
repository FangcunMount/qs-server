package ability

import "github.com/fangcun-mount/qs-server/internal/pkg/calculation"

// CalculationAbility 计算能力
type CalculationAbility struct {
	calculationRule *calculation.CalculationRule
}

// GetCalculationRule 获取计算规则
func (c *CalculationAbility) GetCalculationRule() *calculation.CalculationRule {
	return c.calculationRule
}

// SetCalculationRule 设置计算规则
func (c *CalculationAbility) SetCalculationRule(calculationRule *calculation.CalculationRule) {
	c.calculationRule = calculationRule
}

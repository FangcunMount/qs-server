package ability

// CalculationAbility 计算能力
type CalculationAbility struct {
	calculationRule *CalculationRule
}

// GetCalculationRule 获取计算规则
func (c *CalculationAbility) GetCalculationRule() *CalculationRule {
	return c.calculationRule
}

// SetCalculationRule 设置计算规则
func (c *CalculationAbility) SetCalculationRule(calculationRule *CalculationRule) {
	c.calculationRule = calculationRule
}

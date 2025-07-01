package vo

// FormulaType 公式类型
type FormulaType string

const (
	FormulaTypeScore FormulaType = "score" // 选项分值
	FormulaTypeSum   FormulaType = "sum"   // 求和
	FormulaTypeAvg   FormulaType = "avg"   // 平均值
	FormulaTypeMax   FormulaType = "max"   // 最大值
	FormulaTypeMin   FormulaType = "min"   // 最小值
)

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

// CalculationRule 计算规则
type CalculationRule struct {
	formula FormulaType
}

// GetFormulaType 获取公式类型
func (c *CalculationRule) GetFormulaType() FormulaType {
	return c.formula
}

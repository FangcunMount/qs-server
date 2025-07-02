package calculation

// FormulaType 公式类型
type FormulaType string

const (
	FormulaTypeScore FormulaType = "score" // 选项分值
	FormulaTypeSum   FormulaType = "sum"   // 求和
	FormulaTypeAvg   FormulaType = "avg"   // 平均值
	FormulaTypeMax   FormulaType = "max"   // 最大值
	FormulaTypeMin   FormulaType = "min"   // 最小值
)

// CalculationRule 计算规则
type CalculationRule struct {
	formula FormulaType
}

// NewCalculationRule 创建计算规则
func NewCalculationRule(formula FormulaType) *CalculationRule {
	return &CalculationRule{
		formula: formula,
	}
}

// GetFormulaType 获取公式类型
func (c *CalculationRule) GetFormulaType() FormulaType {
	return c.formula
}

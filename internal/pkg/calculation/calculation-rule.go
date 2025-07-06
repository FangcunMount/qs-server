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

// String 实现 Stringer 接口
func (f FormulaType) String() string {
	return string(f)
}

// CalculationRule 计算规则
type CalculationRule struct {
	formula     FormulaType
	sourceCodes []string
}

// NewCalculationRule 创建计算规则
func NewCalculationRule(formula FormulaType, sourceCodes []string) *CalculationRule {
	return &CalculationRule{
		formula:     formula,
		sourceCodes: sourceCodes,
	}
}

// GetFormulaType 获取公式类型
func (c *CalculationRule) GetFormula() FormulaType {
	return c.formula
}

// GetSourceCodes 获取源码
func (c *CalculationRule) GetSourceCodes() []string {
	return c.sourceCodes
}

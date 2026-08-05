package calculation

// ==================== 计分策略类型 ====================

// StrategyType 计分策略类型（infra/ruleengine 内部实现名）。
// 声明空间 / OpenAPI / Definition 使用 capability catalog 的 canonical 码
// （avg、cnt）；本类型保留 average、count 作为内部名，由 ScaleFactorScorer
// 经 capability.Canonical 映射。公开 ScaleFactorScorer 仅注册 question_aggregation 子集。
type StrategyType string

const (
	// StrategyTypeSum 求和计分：将所有值相加
	StrategyTypeSum StrategyType = "sum"

	// StrategyTypeAverage 平均分计分：内部名 average；声明空间 canonical 为 avg
	StrategyTypeAverage StrategyType = "average"

	// StrategyTypeCount 计数：内部名 count；声明空间 canonical 为 cnt
	StrategyTypeCount StrategyType = "count"
)

// ==================== 计算公式类型（用于规则配置）====================

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

// ==================== 计算规则值对象 ====================

// CalculationRule 计算规则值对象
// 用于配置问题/因子的计分方式
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

// GetFormula 获取公式类型
func (c *CalculationRule) GetFormula() FormulaType {
	return c.formula
}

// GetSourceCodes 获取源码列表
func (c *CalculationRule) GetSourceCodes() []string {
	return c.sourceCodes
}

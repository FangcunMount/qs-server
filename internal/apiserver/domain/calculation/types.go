package calculation

// ==================== 计分策略类型 ====================

// StrategyType 计分策略类型（infra/ruleengine 内部实现名）。
// 声明空间 / OpenAPI / Definition 使用 capability catalog 的 canonical 码
//（avg、cnt）；本类型保留 average、count 作为内部名，由 ScaleFactorScorer
// 经 capability.Canonical 映射。公开 ScaleFactorScorer 仅注册 question_aggregation 子集。
type StrategyType string

const (
	// StrategyTypeSum 求和计分：将所有值相加
	StrategyTypeSum StrategyType = "sum"

	// StrategyTypeAverage 平均分计分：内部名 average；声明空间 canonical 为 avg
	StrategyTypeAverage StrategyType = "average"

	// StrategyTypeWeightedSum 加权求和：按权重加权求和（composite projection；非 ScaleFactorScorer 公开面）
	StrategyTypeWeightedSum StrategyType = "weighted_sum"

	// StrategyTypeMax 最大值（legacy helper；不在声明空间 / 公开 ScaleFactorScorer）
	StrategyTypeMax StrategyType = "max"

	// StrategyTypeMin 最小值（legacy helper；不在声明空间 / 公开 ScaleFactorScorer）
	StrategyTypeMin StrategyType = "min"

	// StrategyTypeCount 计数：内部名 count；声明空间 canonical 为 cnt
	StrategyTypeCount StrategyType = "count"

	// StrategyTypeFirst 取第一个值（legacy helper；不在声明空间）
	StrategyTypeFirst StrategyType = "first"

	// StrategyTypeLast 取最后一个值（legacy helper；不在声明空间）
	StrategyTypeLast StrategyType = "last"
)

// String 返回策略类型的字符串表示
func (s StrategyType) String() string {
	return string(s)
}

// IsValid 检查策略类型是否有效
func (s StrategyType) IsValid() bool {
	switch s {
	case StrategyTypeSum, StrategyTypeAverage, StrategyTypeWeightedSum,
		StrategyTypeMax, StrategyTypeMin, StrategyTypeCount,
		StrategyTypeFirst, StrategyTypeLast:
		return true
	default:
		return false
	}
}

// ==================== 计分参数键名常量 ====================

const (
	// ParamKeyWeights 权重参数键（JSON 数组格式，如 "[0.5, 0.3, 0.2]"）
	ParamKeyWeights = "weights"

	// ParamKeyPrecision 精度参数键（小数位数）
	ParamKeyPrecision = "precision"

	// ParamKeyReverseMax 反向计分最大值参数键
	ParamKeyReverseMax = "reverse_max"

	// ParamKeyReverseMin 反向计分最小值参数键
	ParamKeyReverseMin = "reverse_min"
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

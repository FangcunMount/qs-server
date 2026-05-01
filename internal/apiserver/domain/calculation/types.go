package calculation

// ==================== 计分策略类型 ====================

// StrategyType 计分策略类型
type StrategyType string

const (
	// StrategyTypeSum 求和计分：将所有值相加
	StrategyTypeSum StrategyType = "sum"

	// StrategyTypeAverage 平均分计分：所有值的平均值
	StrategyTypeAverage StrategyType = "average"

	// StrategyTypeWeightedSum 加权求和：按权重加权求和
	StrategyTypeWeightedSum StrategyType = "weighted_sum"

	// StrategyTypeMax 最大值：取所有值的最大值
	StrategyTypeMax StrategyType = "max"

	// StrategyTypeMin 最小值：取所有值的最小值
	StrategyTypeMin StrategyType = "min"

	// StrategyTypeCount 计数：统计值的数量
	StrategyTypeCount StrategyType = "count"

	// StrategyTypeFirst 取第一个值
	StrategyTypeFirst StrategyType = "first"

	// StrategyTypeLast 取最后一个值
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

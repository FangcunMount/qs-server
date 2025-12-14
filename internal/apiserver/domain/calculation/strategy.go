package calculation

// ==================== 计分策略接口 ====================

// ScoringStrategy 计分策略接口
// 每种计分策略类型对应一个策略实现
// 设计原则：无状态，纯函数式计算
type ScoringStrategy interface {
	// Calculate 执行计分计算
	// values: 待计算的值列表
	// params: 计分参数（如权重、精度等）
	// 返回：计算结果和可能的错误
	Calculate(values []float64, params map[string]string) (float64, error)

	// StrategyType 返回策略类型
	StrategyType() StrategyType
}

// ==================== 策略注册表 ====================

// strategyRegistry 策略注册表
var strategyRegistry = make(map[StrategyType]ScoringStrategy)

// RegisterStrategy 注册计分策略
func RegisterStrategy(strategy ScoringStrategy) {
	strategyRegistry[strategy.StrategyType()] = strategy
}

// GetStrategy 获取计分策略
func GetStrategy(strategyType StrategyType) ScoringStrategy {
	return strategyRegistry[strategyType]
}

// HasStrategy 检查策略是否已注册
func HasStrategy(strategyType StrategyType) bool {
	_, ok := strategyRegistry[strategyType]
	return ok
}

// ==================== 初始化注册 ====================

func init() {
	RegisterStrategy(&SumStrategy{})
	RegisterStrategy(&AverageStrategy{})
	RegisterStrategy(&WeightedSumStrategy{})
	RegisterStrategy(&MaxStrategy{})
	RegisterStrategy(&MinStrategy{})
	RegisterStrategy(&CountStrategy{})
	RegisterStrategy(&FirstStrategy{})
	RegisterStrategy(&LastStrategy{})
}

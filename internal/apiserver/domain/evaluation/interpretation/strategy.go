package interpretation

// ==================== 解读策略接口 ====================

// InterpretStrategy 解读策略接口
// 每种解读策略类型对应一个策略实现
// 设计原则：无状态，纯函数式解读
type InterpretStrategy interface {
	// Interpret 执行解读
	// score: 待解读的得分
	// config: 解读配置（包含规则列表）
	// 返回：解读结果和可能的错误
	Interpret(score float64, config *InterpretConfig) (*InterpretResult, error)

	// StrategyType 返回策略类型
	StrategyType() StrategyType
}

// CompositeStrategy 组合解读策略接口
// 支持多因子组合解读
type CompositeStrategy interface {
	// InterpretMultiple 执行多因子组合解读
	// scores: 各因子得分列表
	// config: 组合解读配置
	// 返回：组合解读结果和可能的错误
	InterpretMultiple(scores []FactorScore, config *CompositeConfig) (*CompositeResult, error)

	// StrategyType 返回策略类型
	StrategyType() StrategyType
}

// ==================== 策略注册表 ====================

// strategyRegistry 解读策略注册表
var strategyRegistry = make(map[StrategyType]InterpretStrategy)

// compositeRegistry 组合解读策略注册表
var compositeRegistry = make(map[StrategyType]CompositeStrategy)

// RegisterStrategy 注册解读策略
func RegisterStrategy(strategy InterpretStrategy) {
	strategyRegistry[strategy.StrategyType()] = strategy
}

// GetStrategy 获取解读策略
func GetStrategy(strategyType StrategyType) InterpretStrategy {
	return strategyRegistry[strategyType]
}

// HasStrategy 检查策略是否已注册
func HasStrategy(strategyType StrategyType) bool {
	_, ok := strategyRegistry[strategyType]
	return ok
}

// RegisterCompositeStrategy 注册组合解读策略
func RegisterCompositeStrategy(strategy CompositeStrategy) {
	compositeRegistry[strategy.StrategyType()] = strategy
}

// GetCompositeStrategy 获取组合解读策略
func GetCompositeStrategy(strategyType StrategyType) CompositeStrategy {
	return compositeRegistry[strategyType]
}

// ==================== 初始化注册 ====================

func init() {
	RegisterStrategy(&ThresholdStrategy{})
	RegisterStrategy(&RangeStrategy{})
	RegisterCompositeStrategy(&CompositeStrategyImpl{})
}

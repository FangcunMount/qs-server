package interpretation

import (
	"fmt"
)

// ==================== 解读器接口 ====================

// Interpreter 解读器接口
//
// 设计原则：interpretation 领域只关心"得分 + 规则 → 解读结果"
// 不依赖测评、量表等业务对象
type Interpreter interface {
	// InterpretFactor 解读单个因子得分
	// score: 因子得分
	// config: 解读配置（包含规则列表和策略类型）
	// strategyType: 解读策略类型（threshold、range 等）
	// 返回：解读结果
	InterpretFactor(score float64, config *InterpretConfig, strategyType StrategyType) (*InterpretResult, error)

	// InterpretFactorWithRule 使用指定规则解读因子得分（单规则快速解读）
	// score: 因子得分
	// rule: 单条解读规则
	// 返回：解读结果，若不匹配返回 nil
	InterpretFactorWithRule(score float64, rule InterpretRule) *InterpretResult

	// InterpretMultipleFactors 解读多个因子（组合解读）
	// scores: 各因子得分列表
	// config: 组合解读配置
	// strategyType: 组合解读策略类型
	// 返回：组合解读结果
	InterpretMultipleFactors(scores []FactorScore, config *CompositeConfig, strategyType StrategyType) (*CompositeResult, error)
}

// ==================== 默认解读器实现 ====================

// DefaultInterpreter 默认解读器
type DefaultInterpreter struct{}

// NewDefaultInterpreter 创建默认解读器
func NewDefaultInterpreter() *DefaultInterpreter {
	return &DefaultInterpreter{}
}

// InterpretFactor 解读单个因子得分
func (i *DefaultInterpreter) InterpretFactor(
	score float64,
	config *InterpretConfig,
	strategyType StrategyType,
) (*InterpretResult, error) {
	if config == nil {
		return nil, fmt.Errorf("interpret config is nil")
	}

	// 获取对应的解读策略
	strategy := GetStrategy(strategyType)
	if strategy == nil {
		return nil, fmt.Errorf("unsupported strategy type: %s", strategyType)
	}

	// 执行解读
	result, err := strategy.Interpret(score, config)
	if err != nil {
		return nil, fmt.Errorf("interpret failed: %w", err)
	}

	return result, nil
}

// InterpretFactorWithRule 使用指定规则解读因子得分
func (i *DefaultInterpreter) InterpretFactorWithRule(
	score float64,
	rule InterpretRule,
) *InterpretResult {
	// 检查得分是否在规则区间内
	if !rule.Contains(score) {
		return nil
	}

	// 构造解读结果
	return &InterpretResult{
		Score:       score,
		RiskLevel:   rule.RiskLevel,
		Label:       rule.Label,
		Description: rule.Description,
		Suggestion:  rule.Suggestion,
	}
}

// InterpretMultipleFactors 解读多个因子（组合解读）
func (i *DefaultInterpreter) InterpretMultipleFactors(
	scores []FactorScore,
	config *CompositeConfig,
	strategyType StrategyType,
) (*CompositeResult, error) {
	if config == nil {
		return nil, fmt.Errorf("composite config is nil")
	}

	// 获取对应的组合解读策略
	strategy := GetCompositeStrategy(strategyType)
	if strategy == nil {
		return nil, fmt.Errorf("unsupported composite strategy type: %s", strategyType)
	}

	// 执行组合解读
	result, err := strategy.InterpretMultiple(scores, config)
	if err != nil {
		return nil, fmt.Errorf("composite interpret failed: %w", err)
	}

	return result, nil
}

// ==================== 默认解读器实例 ====================

// 默认解读器（单例）
var defaultInterpreter = NewDefaultInterpreter()

// DefaultInterpreter 获取默认解读器
func GetDefaultInterpreter() *DefaultInterpreter {
	return defaultInterpreter
}

// ==================== 便捷函数 ====================

// InterpretFactor 使用默认解读器解读因子得分（便捷函数）
func InterpretFactor(score float64, config *InterpretConfig, strategyType StrategyType) (*InterpretResult, error) {
	return defaultInterpreter.InterpretFactor(score, config, strategyType)
}

// InterpretFactorWithRule 使用默认解读器根据单规则解读因子得分（便捷函数）
func InterpretFactorWithRule(score float64, rule InterpretRule) *InterpretResult {
	return defaultInterpreter.InterpretFactorWithRule(score, rule)
}

// InterpretMultipleFactors 使用默认解读器解读多个因子（便捷函数）
func InterpretMultipleFactors(scores []FactorScore, config *CompositeConfig, strategyType StrategyType) (*CompositeResult, error) {
	return defaultInterpreter.InterpretMultipleFactors(scores, config, strategyType)
}

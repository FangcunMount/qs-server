package strategies

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/collection-server/domain/validation/rules"
)

// ValidationStrategy 验证策略接口
type ValidationStrategy interface {
	Validate(value interface{}, rule *rules.BaseRule) error
	GetStrategyName() string
}

// BaseStrategy 基础验证策略
type BaseStrategy struct {
	Name string
}

// GetStrategyName 获取策略名称
func (s *BaseStrategy) GetStrategyName() string {
	return s.Name
}

// StrategyFactory 验证策略工厂
type StrategyFactory struct {
	strategies map[string]ValidationStrategy
}

// NewStrategyFactory 创建策略工厂
func NewStrategyFactory() *StrategyFactory {
	factory := &StrategyFactory{
		strategies: make(map[string]ValidationStrategy),
	}

	// 注册默认策略
	factory.RegisterDefaultStrategies()

	return factory
}

// RegisterStrategy 注册验证策略
func (f *StrategyFactory) RegisterStrategy(strategy ValidationStrategy) {
	f.strategies[strategy.GetStrategyName()] = strategy
}

// GetStrategy 获取验证策略
func (f *StrategyFactory) GetStrategy(name string) (ValidationStrategy, error) {
	strategy, exists := f.strategies[name]
	if !exists {
		return nil, fmt.Errorf("验证策略 '%s' 不存在", name)
	}
	return strategy, nil
}

// RegisterDefaultStrategies 注册默认验证策略
func (f *StrategyFactory) RegisterDefaultStrategies() {
	f.RegisterStrategy(NewRequiredStrategy())
	f.RegisterStrategy(NewMinValueStrategy())
	f.RegisterStrategy(NewMaxValueStrategy())
	f.RegisterStrategy(NewMinLengthStrategy())
	f.RegisterStrategy(NewMaxLengthStrategy())
	f.RegisterStrategy(NewPatternStrategy())
	f.RegisterStrategy(NewEmailStrategy())
}

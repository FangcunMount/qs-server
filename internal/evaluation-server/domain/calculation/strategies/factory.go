package strategies

import (
	"fmt"
	"sync"
)

// StrategyFactory 计算策略工厂
type StrategyFactory struct {
	strategies map[string]CalculationStrategy
	mu         sync.RWMutex
}

// NewStrategyFactory 创建策略工厂
func NewStrategyFactory() *StrategyFactory {
	factory := &StrategyFactory{
		strategies: make(map[string]CalculationStrategy),
	}

	// 注册默认策略
	factory.RegisterDefaultStrategies()

	return factory
}

// RegisterStrategy 注册计算策略
func (f *StrategyFactory) RegisterStrategy(strategy CalculationStrategy) error {
	if strategy == nil {
		return fmt.Errorf("策略不能为空")
	}

	name := strategy.GetStrategyName()
	if name == "" {
		return fmt.Errorf("策略名称不能为空")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.strategies[name] = strategy
	return nil
}

// GetStrategy 获取计算策略
func (f *StrategyFactory) GetStrategy(name string) (CalculationStrategy, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	strategy, exists := f.strategies[name]
	if !exists {
		return nil, fmt.Errorf("计算策略 '%s' 不存在", name)
	}

	return strategy, nil
}

// ListStrategies 列出所有策略
func (f *StrategyFactory) ListStrategies() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	names := make([]string, 0, len(f.strategies))
	for name := range f.strategies {
		names = append(names, name)
	}

	return names
}

// HasStrategy 检查策略是否存在
func (f *StrategyFactory) HasStrategy(name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	_, exists := f.strategies[name]
	return exists
}

// RegisterDefaultStrategies 注册默认策略
func (f *StrategyFactory) RegisterDefaultStrategies() {
	f.RegisterStrategy(NewSumStrategy())
	f.RegisterStrategy(NewAverageStrategy())
	f.RegisterStrategy(NewMaxStrategy())
	f.RegisterStrategy(NewMinStrategy())
	f.RegisterStrategy(NewOptionStrategy())
	f.RegisterStrategy(NewWeightedStrategy())
}

// 全局策略工厂实例
var (
	globalFactory *StrategyFactory
	once          sync.Once
)

// GetGlobalStrategyFactory 获取全局策略工厂实例
func GetGlobalStrategyFactory() *StrategyFactory {
	once.Do(func() {
		globalFactory = NewStrategyFactory()
	})
	return globalFactory
}

// RegisterCustomStrategy 注册自定义计算策略
func RegisterCustomStrategy(strategy CalculationStrategy) error {
	factory := GetGlobalStrategyFactory()
	return factory.RegisterStrategy(strategy)
}

// GetStrategy 获取计算策略
func GetStrategy(name string) (CalculationStrategy, error) {
	factory := GetGlobalStrategyFactory()
	return factory.GetStrategy(name)
}

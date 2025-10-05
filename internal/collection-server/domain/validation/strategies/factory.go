package strategies

import (
	"sync"

	"github.com/fangcun-mount/qs-server/internal/collection-server/domain/validation/rules"
)

// GlobalStrategyFactory 全局策略工厂实例
var (
	globalFactory *StrategyFactory
	once          sync.Once
)

// GetGlobalStrategyFactory 获取全局策略工厂实例（单例模式）
func GetGlobalStrategyFactory() *StrategyFactory {
	once.Do(func() {
		globalFactory = NewStrategyFactory()
	})
	return globalFactory
}

// RegisterCustomStrategy 注册自定义验证策略
func RegisterCustomStrategy(strategy ValidationStrategy) error {
	factory := GetGlobalStrategyFactory()
	factory.RegisterStrategy(strategy)
	return nil
}

// GetStrategy 获取验证策略
func GetStrategy(name string) (ValidationStrategy, error) {
	factory := GetGlobalStrategyFactory()
	return factory.GetStrategy(name)
}

// ValidateWithStrategy 使用指定策略验证值
func ValidateWithStrategy(strategyName string, value interface{}, rule *rules.BaseRule) error {
	strategy, err := GetStrategy(strategyName)
	if err != nil {
		return err
	}
	return strategy.Validate(value, rule)
}

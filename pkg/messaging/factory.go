package messaging

import (
	"fmt"
	"sync"
)

// Provider 消息中间件提供者类型
type Provider string

const (
	// ProviderNSQ NSQ 消息队列
	ProviderNSQ Provider = "nsq"

	// ProviderRabbitMQ RabbitMQ 消息队列
	ProviderRabbitMQ Provider = "rabbitmq"
)

// EventBusFactory 事件总线工厂函数
// 接收配置，返回 EventBus 实例
type EventBusFactory func(config *Config) (EventBus, error)

var (
	// 全局工厂注册表
	factoryRegistry = make(map[Provider]EventBusFactory)
	registryMu      sync.RWMutex
)

// RegisterProvider 注册消息中间件提供者
// 这个函数由各个实现包（nsq、rabbitmq）在 init 函数中调用
//
// 示例：
//
//	func init() {
//	    messaging.RegisterProvider(messaging.ProviderNSQ, NewEventBusFromConfig)
//	}
func RegisterProvider(provider Provider, factory EventBusFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if factory == nil {
		panic("messaging: Register factory is nil for provider " + provider)
	}

	if _, exists := factoryRegistry[provider]; exists {
		panic("messaging: Register called twice for provider " + provider)
	}

	factoryRegistry[provider] = factory
}

// NewEventBus 创建事件总线
// 这是对外暴露的统一接口，根据配置中的 Provider 选择具体实现
//
// 使用示例：
//
//	config := &messaging.Config{
//	    Provider: messaging.ProviderNSQ,
//	    NSQ: messaging.NSQConfig{...},
//	}
//	bus, err := messaging.NewEventBus(config)
func NewEventBus(config *Config) (EventBus, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	registryMu.RLock()
	factory, exists := factoryRegistry[config.Provider]
	registryMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unsupported messaging provider: %s (available: %v)",
			config.Provider, GetRegisteredProviders())
	}

	return factory(config)
}

// GetRegisteredProviders 获取已注册的提供者列表
func GetRegisteredProviders() []Provider {
	registryMu.RLock()
	defer registryMu.RUnlock()

	providers := make([]Provider, 0, len(factoryRegistry))
	for provider := range factoryRegistry {
		providers = append(providers, provider)
	}
	return providers
}

// MustNewEventBus 创建事件总线，失败则 panic
// 适用于启动阶段，配置错误时快速失败
func MustNewEventBus(config *Config) EventBus {
	bus, err := NewEventBus(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create event bus: %v", err))
	}
	return bus
}

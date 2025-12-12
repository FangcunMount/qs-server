// Package handlers 提供事件处理器注册机制
//
// 使用 init() 模式自动注册处理器：
//
//	func init() {
//	    handlers.Register("my_handler", func(deps *handlers.Dependencies) handlers.HandlerFunc {
//	        return func(ctx context.Context, eventType string, payload []byte) error {
//	            // 处理逻辑
//	            return nil
//	        }
//	    })
//	}
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	redis "github.com/redis/go-redis/v9"
)

// HandlerFunc 处理器函数类型
type HandlerFunc func(ctx context.Context, eventType string, payload []byte) error

// Dependencies 处理器依赖
type Dependencies struct {
	Logger            *slog.Logger
	AnswerSheetClient *grpcclient.AnswerSheetClient
	EvaluationClient  *grpcclient.EvaluationClient
	InternalClient    *grpcclient.InternalClient
	RedisCache        redis.UniversalClient
}

// HandlerFactory 处理器工厂函数
// 接收依赖，返回处理器函数
type HandlerFactory func(deps *Dependencies) HandlerFunc

// ==================== 事件消息解析 ====================

// EventEnvelope 事件信封结构
// 对应发布端 event.Event[T] 的 JSON 序列化格式
type EventEnvelope struct {
	ID            string          `json:"id"`
	EventType     string          `json:"eventType"`
	OccurredAt    time.Time       `json:"occurredAt"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   string          `json:"aggregateID"`
	Data          json.RawMessage `json:"data"` // 业务数据，延迟解析
}

// ParseEventEnvelope 解析事件信封
func ParseEventEnvelope(payload []byte) (*EventEnvelope, error) {
	var env EventEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, fmt.Errorf("failed to parse event envelope: %w", err)
	}
	return &env, nil
}

// ParseEventData 解析事件业务数据到指定类型
// 用法: var data MyPayload; ParseEventData(payload, &data)
func ParseEventData[T any](payload []byte, target *T) (*EventEnvelope, error) {
	env, err := ParseEventEnvelope(payload)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(env.Data, target); err != nil {
		return nil, fmt.Errorf("failed to parse event data: %w", err)
	}

	return env, nil
}

// ==================== 全局注册表 ====================

var (
	registryMu sync.RWMutex
	registry   = make(map[string]HandlerFactory)
)

// Register 注册处理器工厂
// 在 init() 中调用，注册处理器名称与工厂函数的映射
func Register(name string, factory HandlerFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("handler %q already registered", name))
	}
	registry[name] = factory
}

// GetFactory 获取处理器工厂
func GetFactory(name string) (HandlerFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	factory, ok := registry[name]
	return factory, ok
}

// ListRegistered 列出所有已注册的处理器名称
func ListRegistered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// CreateAll 根据依赖创建所有已注册的处理器
func CreateAll(deps *Dependencies) map[string]HandlerFunc {
	registryMu.RLock()
	defer registryMu.RUnlock()

	handlers := make(map[string]HandlerFunc, len(registry))
	for name, factory := range registry {
		handlers[name] = factory(deps)
	}
	return handlers
}

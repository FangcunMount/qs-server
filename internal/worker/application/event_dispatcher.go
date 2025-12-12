// Package application 提供 Worker 的应用层服务
// 负责事件订阅、分发和处理器编排
package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	redis "github.com/redis/go-redis/v9"
)

// EventDispatcher 事件分发器
// 负责：
// 1. 加载事件配置
// 2. 注册处理器
// 3. 订阅 Topic
// 4. 分发事件到对应处理器
type EventDispatcher struct {
	logger     *slog.Logger
	subscriber *eventconfig.Subscriber
	registry   *eventconfig.Registry

	// 处理器依赖
	deps *HandlerDependencies
}

// HandlerDependencies 处理器依赖
type HandlerDependencies struct {
	Logger            *slog.Logger
	AnswerSheetClient *grpcclient.AnswerSheetClient
	EvaluationClient  *grpcclient.EvaluationClient
	InternalClient    *grpcclient.InternalClient
	RedisCache        redis.UniversalClient
}

// NewEventDispatcher 创建事件分发器
func NewEventDispatcher(logger *slog.Logger, deps *HandlerDependencies) *EventDispatcher {
	return &EventDispatcher{
		logger: logger,
		deps:   deps,
	}
}

// Initialize 初始化事件分发器
// 1. 加载事件配置
// 2. 创建处理器工厂（使用 init() 自注册机制）
// 3. 注册所有处理器
func (d *EventDispatcher) Initialize(configPath string) error {
	d.logger.Info("initializing event dispatcher",
		slog.String("config_path", configPath),
	)

	// 1. 加载事件配置
	if err := eventconfig.Initialize(configPath); err != nil {
		return fmt.Errorf("failed to load event config: %w", err)
	}
	d.registry = eventconfig.Global()

	// 2. 列出已通过 init() 注册的处理器
	registeredHandlers := handlers.ListRegistered()
	d.logger.Info("handlers registered via init()",
		slog.Int("count", len(registeredHandlers)),
		slog.Any("handlers", registeredHandlers),
	)

	// 3. 将依赖转换为 handlers.Dependencies
	handlerDeps := &handlers.Dependencies{
		Logger:            d.deps.Logger,
		AnswerSheetClient: d.deps.AnswerSheetClient,
		EvaluationClient:  d.deps.EvaluationClient,
		InternalClient:    d.deps.InternalClient,
		RedisCache:        d.deps.RedisCache,
	}

	// 4. 创建处理器工厂（基于 init() 注册的处理器）
	factory := d.createHandlerFactory(handlerDeps)

	// 5. 创建订阅器
	d.subscriber = eventconfig.NewSubscriber(eventconfig.SubscriberOptions{
		HandlerFactory: factory,
		Logger:         d.logger,
	})

	// 6. 注册处理器
	if err := d.subscriber.RegisterHandlers(); err != nil {
		return fmt.Errorf("failed to register handlers: %w", err)
	}

	d.logger.Info("event dispatcher initialized",
		slog.Int("handler_count", d.subscriber.HandlerCount()),
	)

	return nil
}

// createHandlerFactory 创建处理器工厂
// 使用 handlers 包的 init() 自注册机制
func (d *EventDispatcher) createHandlerFactory(deps *handlers.Dependencies) eventconfig.HandlerFactory {
	// 预先创建所有处理器实例
	allHandlers := handlers.CreateAll(deps)

	// 返回工厂函数
	return func(handlerName string) (eventconfig.HandlerFunc, error) {
		handler, ok := allHandlers[handlerName]
		if !ok {
			return nil, fmt.Errorf("handler %q not registered via init()", handlerName)
		}
		// 适配 handlers.HandlerFunc -> eventconfig.HandlerFunc
		return eventconfig.HandlerFunc(handler), nil
	}
}

// GetTopicSubscriptions 获取需要订阅的 Topic 列表
func (d *EventDispatcher) GetTopicSubscriptions() []eventconfig.TopicSubscription {
	if d.subscriber == nil {
		return nil
	}
	return d.subscriber.GetTopicsToSubscribe()
}

// Dispatch 分发事件到对应的处理器
func (d *EventDispatcher) Dispatch(ctx context.Context, eventType string, payload []byte) error {
	if d.subscriber == nil {
		return fmt.Errorf("event dispatcher not initialized")
	}
	return d.subscriber.Dispatch(ctx, eventType, payload)
}

// HasHandler 检查是否有处理器
func (d *EventDispatcher) HasHandler(eventType string) bool {
	if d.subscriber == nil {
		return false
	}
	return d.subscriber.HasHandler(eventType)
}

// PrintSubscriptionInfo 打印订阅信息（用于启动日志）
func (d *EventDispatcher) PrintSubscriptionInfo() {
	subs := d.GetTopicSubscriptions()

	d.logger.Info("=== Topic Subscriptions ===")
	for _, sub := range subs {
		d.logger.Info("topic subscription",
			slog.String("topic", sub.TopicName),
			slog.String("group", sub.Group),
			slog.Int("concurrency", sub.Concurrency),
			slog.Int("event_count", len(sub.EventTypes)),
		)
		for _, eventType := range sub.EventTypes {
			hasHandler := "✗"
			if d.HasHandler(eventType) {
				hasHandler = "✓"
			}
			d.logger.Info("  event type",
				slog.String("event_type", eventType),
				slog.String("has_handler", hasHandler),
			)
		}
	}
}

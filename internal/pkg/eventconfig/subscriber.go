package eventconfig

import (
	"context"
	"fmt"
	"log/slog"
)

// HandlerFunc 处理器函数类型
type HandlerFunc func(ctx context.Context, eventType string, payload []byte) error

// HandlerFactory 处理器工厂函数
// 根据处理器名称创建处理器实例
type HandlerFactory func(handlerName string) (HandlerFunc, error)

// Subscriber 配置驱动的事件订阅器
// 根据配置自动订阅 Topic 并分发事件
type Subscriber struct {
	registry       *Registry
	handlerFactory HandlerFactory
	logger         *slog.Logger

	// 缓存：事件类型 -> 处理器
	handlers map[string]HandlerFunc
}

// SubscriberOptions 订阅器选项
type SubscriberOptions struct {
	Registry       *Registry
	HandlerFactory HandlerFactory
	Logger         *slog.Logger
}

// NewSubscriber 创建订阅器
func NewSubscriber(opts SubscriberOptions) *Subscriber {
	if opts.Registry == nil {
		opts.Registry = Global()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &Subscriber{
		registry:       opts.Registry,
		handlerFactory: opts.HandlerFactory,
		logger:         opts.Logger,
		handlers:       make(map[string]HandlerFunc),
	}
}

// RegisterHandlers 根据配置注册所有处理器
func (s *Subscriber) RegisterHandlers() error {
	cfg := s.registry.Config()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	for eventType, eventCfg := range cfg.Events {
		if eventCfg.Handler == "" {
			continue
		}

		handler, err := s.handlerFactory(eventCfg.Handler)
		if err != nil {
			s.logger.Warn("handler not found, skipping",
				slog.String("event_type", eventType),
				slog.String("handler", eventCfg.Handler),
				slog.String("error", err.Error()),
			)
			continue
		}

		s.handlers[eventType] = handler
		s.logger.Info("handler registered",
			slog.String("event_type", eventType),
			slog.String("handler", eventCfg.Handler),
		)
	}

	return nil
}

// GetTopicsToSubscribe 获取需要订阅的 Topic 列表
func (s *Subscriber) GetTopicsToSubscribe() []TopicSubscription {
	cfg := s.registry.Config()
	if cfg == nil {
		return nil
	}

	var subs []TopicSubscription
	for topicKey, topicCfg := range cfg.Topics {
		events := cfg.GetEventsByTopic(topicKey)
		if len(events) == 0 {
			continue
		}

		subs = append(subs, TopicSubscription{
			TopicName:   topicCfg.Name,
			TopicKey:    topicKey,
			Group:       topicCfg.Consumer.Group,
			Concurrency: topicCfg.Consumer.Concurrency,
			EventTypes:  events,
		})
	}

	return subs
}

// TopicSubscription Topic 订阅信息
type TopicSubscription struct {
	TopicName   string
	TopicKey    string
	Group       string
	Concurrency int
	EventTypes  []string
}

// Dispatch 分发事件到对应的处理器
func (s *Subscriber) Dispatch(ctx context.Context, eventType string, payload []byte) error {
	handler, ok := s.handlers[eventType]
	if !ok {
		s.logger.Warn("no handler for event type",
			slog.String("event_type", eventType),
		)
		return nil // 没有处理器不算错误
	}

	return handler(ctx, eventType, payload)
}

// HasHandler 检查是否有处理器
func (s *Subscriber) HasHandler(eventType string) bool {
	_, ok := s.handlers[eventType]
	return ok
}

// HandlerCount 返回已注册的处理器数量
func (s *Subscriber) HandlerCount() int {
	return len(s.handlers)
}

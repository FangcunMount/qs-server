package eventconfig

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

// HandlerFunc 处理器函数类型
type HandlerFunc func(ctx context.Context, eventType string, payload []byte) error

// HandlerFactory 处理器工厂函数
// 根据处理器名称创建处理器实例
type HandlerFactory func(handlerName string) (HandlerFunc, error)

// Subscriber 配置驱动的事件订阅器
// 根据配置自动订阅 Topic 并分发事件
type Subscriber struct {
	catalog        catalogReader
	handlerFactory HandlerFactory
	logger         *slog.Logger

	// 缓存：事件类型 -> 处理器
	handlers map[string]HandlerFunc
}

// SubscriberOptions 订阅器选项
type SubscriberOptions struct {
	Registry       *Registry
	Catalog        *eventcatalog.Catalog
	HandlerFactory HandlerFactory
	Logger         *slog.Logger
}

type catalogReader interface {
	Config() *eventcatalog.Config
	TopicSubscriptions() []eventcatalog.TopicSubscription
}

// NewSubscriber 创建订阅器
func NewSubscriber(opts SubscriberOptions) *Subscriber {
	catalog := catalogReader(nil)
	if opts.Catalog != nil {
		catalog = opts.Catalog
	} else if opts.Registry != nil {
		catalog = opts.Registry
	} else {
		catalog = Global()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &Subscriber{
		catalog:        catalog,
		handlerFactory: opts.HandlerFactory,
		logger:         opts.Logger,
		handlers:       make(map[string]HandlerFunc),
	}
}

// RegisterHandlers 根据配置注册所有处理器
func (s *Subscriber) RegisterHandlers() error {
	cfg := s.catalog.Config()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	for eventType, eventCfg := range cfg.Events {
		handler, err := s.handlerFactory(eventCfg.Handler)
		if err != nil {
			return fmt.Errorf("event %q references unavailable handler %q: %w", eventType, eventCfg.Handler, err)
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
	return s.catalog.TopicSubscriptions()
}

// TopicSubscription Topic 订阅信息
type TopicSubscription = eventcatalog.TopicSubscription

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

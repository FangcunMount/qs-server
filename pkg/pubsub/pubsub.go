package pubsub

import (
	"context"
	"fmt"
)

// Publisher 发布者接口
type Publisher interface {
	// Publish 发布消息到指定主题
	Publish(ctx context.Context, topic string, message interface{}) error
	// Close 关闭发布者
	Close() error
}

// Subscriber 订阅者接口
type Subscriber interface {
	// Subscribe 订阅主题
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	// SubscribeWithRetry 带重试机制的订阅
	SubscribeWithRetry(ctx context.Context, topic string, handler MessageHandler) error
	// Run 启动订阅者（阻塞运行）
	Run(ctx context.Context) error
	// Close 关闭订阅者
	Close() error
	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
}

// MessageHandler 消息处理函数类型
type MessageHandler func(topic string, data []byte) error

// PubSub 发布订阅组合接口
type PubSub interface {
	Publisher() Publisher
	Subscriber() Subscriber
	Close() error
}

// NewPublisher 创建发布者
func NewPublisher(config *Config) (Publisher, error) {
	return newWatermillPublisher(config)
}

// NewSubscriber 创建订阅者
func NewSubscriber(config *Config) (Subscriber, error) {
	return newWatermillSubscriber(config)
}

// NewPubSub 创建发布订阅实例
func NewPubSub(config *Config) (PubSub, error) {
	return newWatermillPubSub(config)
}

// watermillPubSub 组合发布者和订阅者
type watermillPubSub struct {
	publisher  Publisher
	subscriber Subscriber
}

// newWatermillPubSub 创建发布订阅实例
func newWatermillPubSub(config *Config) (*watermillPubSub, error) {
	publisher, err := newWatermillPublisher(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create publisher: %w", err)
	}

	subscriber, err := newWatermillSubscriber(config)
	if err != nil {
		publisher.Close()
		return nil, fmt.Errorf("failed to create subscriber: %w", err)
	}

	return &watermillPubSub{
		publisher:  publisher,
		subscriber: subscriber,
	}, nil
}

// Publisher 返回发布者
func (ps *watermillPubSub) Publisher() Publisher {
	return ps.publisher
}

// Subscriber 返回订阅者
func (ps *watermillPubSub) Subscriber() Subscriber {
	return ps.subscriber
}

// Close 关闭发布订阅实例
func (ps *watermillPubSub) Close() error {
	var errs []error

	if err := ps.publisher.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close publisher: %w", err))
	}

	if err := ps.subscriber.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close subscriber: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing pubsub: %v", errs)
	}

	return nil
}

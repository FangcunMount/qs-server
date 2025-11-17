package nsq

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	"github.com/nsqio/go-nsq"
)

// publisher NSQ 发布者实现
type publisher struct {
	producer *nsq.Producer
	config   *nsq.Config
}

// NewPublisher 创建 NSQ 发布者
// addr: NSQd 地址，格式为 "host:port"
// cfg: NSQ 配置
func NewPublisher(addr string, cfg *nsq.Config) (messaging.Publisher, error) {
	if cfg == nil {
		cfg = nsq.NewConfig()
	}

	producer, err := nsq.NewProducer(addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create NSQ producer: %w", err)
	}

	// 测试连接
	if err := producer.Ping(); err != nil {
		producer.Stop()
		return nil, fmt.Errorf("failed to ping NSQ: %w", err)
	}

	return &publisher{
		producer: producer,
		config:   cfg,
	}, nil
}

// Publish 发布消息
func (p *publisher) Publish(ctx context.Context, topic string, body []byte) error {
	// 这里可以注入 trace / metric / 日志
	// 例如：span := trace.SpanFromContext(ctx)

	if err := p.producer.Publish(topic, body); err != nil {
		return fmt.Errorf("failed to publish message to topic %s: %w", topic, err)
	}

	return nil
}

// PublishMessage 发布消息对象（支持 Metadata）
func (p *publisher) PublishMessage(ctx context.Context, topic string, msg *messaging.Message) error {
	// 直接使用 Payload 作为消息体
	// TODO: 如果需要传递 Metadata，可以序列化成 JSON 格式
	return p.Publish(ctx, topic, msg.Payload)
}

// PublishAsync 异步发布消息
func (p *publisher) PublishAsync(topic string, body []byte, doneChan chan *nsq.ProducerTransaction, args ...interface{}) error {
	return p.producer.PublishAsync(topic, body, doneChan, args...)
}

// MultiPublish 批量发布消息
func (p *publisher) MultiPublish(ctx context.Context, topic string, bodies [][]byte) error {
	if err := p.producer.MultiPublish(topic, bodies); err != nil {
		return fmt.Errorf("failed to multi-publish messages to topic %s: %w", topic, err)
	}
	return nil
}

// DeferredPublish 延迟发布消息
func (p *publisher) DeferredPublish(ctx context.Context, topic string, delay time.Duration, body []byte) error {
	if err := p.producer.DeferredPublish(topic, delay, body); err != nil {
		return fmt.Errorf("failed to deferred-publish message to topic %s: %w", topic, err)
	}
	return nil
}

// Close 关闭发布者
func (p *publisher) Close() error {
	p.producer.Stop()
	return nil
}

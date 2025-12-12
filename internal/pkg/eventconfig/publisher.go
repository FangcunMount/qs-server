package eventconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// RoutingPublisher 配置驱动的路由发布器
// 根据事件配置自动将事件路由到正确的 Topic
type RoutingPublisher struct {
	registry    *Registry
	mqPublisher messaging.Publisher
	source      string
	mode        PublishMode
}

// PublishMode 发布模式
type PublishMode string

const (
	// PublishModeMQ 消息队列模式
	PublishModeMQ PublishMode = "mq"
	// PublishModeLogging 日志模式（开发调试）
	PublishModeLogging PublishMode = "logging"
	// PublishModeNop 空模式（测试）
	PublishModeNop PublishMode = "nop"
)

// PublishModeFromEnv 根据环境名称返回发布模式
func PublishModeFromEnv(env string) PublishMode {
	switch env {
	case "prod", "production":
		return PublishModeMQ
	case "dev", "development":
		return PublishModeLogging
	case "test", "testing":
		return PublishModeNop
	default:
		return PublishModeLogging
	}
}

// RoutingPublisherOptions 发布器选项
type RoutingPublisherOptions struct {
	Registry    *Registry
	MQPublisher messaging.Publisher
	Source      string
	Mode        PublishMode
}

// NewRoutingPublisher 创建路由发布器
func NewRoutingPublisher(opts RoutingPublisherOptions) *RoutingPublisher {
	if opts.Registry == nil {
		opts.Registry = Global()
	}
	if opts.Source == "" {
		opts.Source = event.SourceAPIServer
	}
	if opts.Mode == "" {
		opts.Mode = PublishModeLogging
	}

	return &RoutingPublisher{
		registry:    opts.Registry,
		mqPublisher: opts.MQPublisher,
		source:      opts.Source,
		mode:        opts.Mode,
	}
}

// Publish 发布事件（自动路由到配置的 Topic）
func (p *RoutingPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	eventType := evt.EventType()

	// 从配置中查找 Topic
	topicName, ok := p.registry.GetTopicForEvent(eventType)
	if !ok {
		return fmt.Errorf("event type %q not found in config", eventType)
	}

	switch p.mode {
	case PublishModeMQ:
		return p.publishToMQ(ctx, topicName, evt)
	case PublishModeLogging:
		return p.publishToLog(ctx, topicName, evt)
	case PublishModeNop:
		return nil
	default:
		return p.publishToLog(ctx, topicName, evt)
	}
}

// PublishAll 批量发布事件
func (p *RoutingPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func (p *RoutingPublisher) publishToMQ(ctx context.Context, topicName string, evt event.DomainEvent) error {
	if p.mqPublisher == nil {
		logger.L(ctx).Warnw("MQ publisher is nil, falling back to logging",
			"event_type", evt.EventType(),
			"topic", topicName,
		)
		return p.publishToLog(ctx, topicName, evt)
	}

	// 序列化事件
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// 构建消息
	msg := messaging.NewMessage(evt.EventID(), payload)
	msg.Metadata["event_type"] = evt.EventType()
	msg.Metadata["aggregate_type"] = evt.AggregateType()
	msg.Metadata["aggregate_id"] = evt.AggregateID()
	msg.Metadata["occurred_at"] = evt.OccurredAt().Format("2006-01-02T15:04:05.000Z07:00")
	msg.Metadata["source"] = p.source

	// 发布到配置的 Topic
	if err := p.mqPublisher.PublishMessage(ctx, topicName, msg); err != nil {
		return fmt.Errorf("failed to publish to topic %s: %w", topicName, err)
	}

	logger.L(ctx).Infow("event published",
		"action", "publish_event",
		"event_type", evt.EventType(),
		"event_id", evt.EventID(),
		"topic", topicName,
		"source", p.source,
		"result", "success",
	)

	return nil
}

func (p *RoutingPublisher) publishToLog(ctx context.Context, topicName string, evt event.DomainEvent) error {
	logger.L(ctx).Infow("[DomainEvent]",
		"action", "log_event",
		"event_type", evt.EventType(),
		"event_id", evt.EventID(),
		"aggregate_type", evt.AggregateType(),
		"aggregate_id", evt.AggregateID(),
		"topic", topicName,
		"source", p.source,
	)
	return nil
}

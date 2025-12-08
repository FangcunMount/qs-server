package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// MessagingEventPublisher 消息队列事件发布器
// 实现 event.EventPublisher 接口，将领域事件发布到消息队列
//
// 设计说明：
// - 这是基础设施层的适配器（Adapter）
// - 将领域事件转换为消息队列的消息格式
// - 支持 NSQ、RabbitMQ 等消息中间件（通过 pkg/messaging 抽象）
type MessagingEventPublisher struct {
	publisher messaging.Publisher
}

// NewMessagingEventPublisher 创建消息队列事件发布器
func NewMessagingEventPublisher(publisher messaging.Publisher) *MessagingEventPublisher {
	return &MessagingEventPublisher{
		publisher: publisher,
	}
}

// Publish 发布单个领域事件
func (p *MessagingEventPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	if p.publisher == nil {
		log.Warnf("publisher is nil, skip event publishing: %s", evt.EventType())
		return nil
	}

	// 序列化领域事件
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal event %s: %w", evt.EventType(), err)
	}

	// 构建消息（添加元数据用于链路追踪）
	msg := messaging.NewMessage(evt.EventID(), payload)
	msg.Metadata["event_type"] = evt.EventType()
	msg.Metadata["aggregate_type"] = evt.AggregateType()
	msg.Metadata["aggregate_id"] = evt.AggregateID()
	msg.Metadata["occurred_at"] = evt.OccurredAt().Format("2006-01-02T15:04:05.000Z07:00")

	// 发布到消息队列（topic = 事件类型）
	if err := p.publisher.PublishMessage(ctx, evt.EventType(), msg); err != nil {
		return fmt.Errorf("failed to publish event %s: %w", evt.EventType(), err)
	}

	log.Infof("event published: type=%s, id=%s, aggregate=%s/%s",
		evt.EventType(), evt.EventID(), evt.AggregateType(), evt.AggregateID())

	return nil
}

// PublishAll 批量发布领域事件
func (p *MessagingEventPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

// Close 关闭发布器
func (p *MessagingEventPublisher) Close() error {
	if p.publisher != nil {
		return p.publisher.Close()
	}
	return nil
}

// ==================== 日志事件发布器（用于开发调试） ====================

// LoggingEventPublisher 日志事件发布器
// 仅记录事件日志，不实际发布到消息队列
// 适用于开发调试或不需要消息队列的场景
type LoggingEventPublisher struct{}

// NewLoggingEventPublisher 创建日志事件发布器
func NewLoggingEventPublisher() *LoggingEventPublisher {
	return &LoggingEventPublisher{}
}

// Publish 记录事件日志
func (p *LoggingEventPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	log.Infof("[DomainEvent] type=%s, id=%s, aggregate=%s/%s, occurred_at=%s",
		evt.EventType(),
		evt.EventID(),
		evt.AggregateType(),
		evt.AggregateID(),
		evt.OccurredAt().Format("2006-01-02T15:04:05.000Z07:00"),
	)
	return nil
}

// PublishAll 批量记录事件日志
func (p *LoggingEventPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/FangcunMount/qs-server/pkg/event"
	"github.com/FangcunMount/qs-server/pkg/messaging"
)

// EventPublisher 事件发布器
// 将领域事件发布到消息队列，供 qs-worker 消费
type EventPublisher struct {
	publisher messaging.Publisher
}

// NewEventPublisher 创建事件发布器
func NewEventPublisher(publisher messaging.Publisher) *EventPublisher {
	return &EventPublisher{
		publisher: publisher,
	}
}

// Publish 发布单个领域事件
func (p *EventPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	if p.publisher == nil {
		log.Warnf("publisher is nil, skip event publishing: %s", evt.EventType())
		return nil
	}

	// 序列化领域事件
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal event %s: %w", evt.EventType(), err)
	}

	// 构建消息
	msg := messaging.NewMessage(evt.EventID(), payload)
	msg.Metadata["event_type"] = evt.EventType()
	msg.Metadata["aggregate_type"] = evt.AggregateType()
	msg.Metadata["aggregate_id"] = evt.AggregateID()
	msg.Metadata["occurred_at"] = evt.OccurredAt().Format("2006-01-02T15:04:05.000Z07:00")
	msg.Metadata["source"] = "collection-server"

	// 发布到消息队列
	if err := p.publisher.PublishMessage(ctx, evt.EventType(), msg); err != nil {
		return fmt.Errorf("failed to publish event %s: %w", evt.EventType(), err)
	}

	log.Infof("event published: type=%s, id=%s, aggregate=%s/%s",
		evt.EventType(), evt.EventID(), evt.AggregateType(), evt.AggregateID())

	return nil
}

// PublishAll 批量发布领域事件
func (p *EventPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

// Close 关闭发布器
func (p *EventPublisher) Close() error {
	if p.publisher != nil {
		return p.publisher.Close()
	}
	return nil
}

// ==================== LoggingEventPublisher（开发调试用）====================

// LoggingEventPublisher 日志事件发布器
// 仅记录日志，不实际发布消息，用于开发调试
type LoggingEventPublisher struct{}

// NewLoggingEventPublisher 创建日志事件发布器
func NewLoggingEventPublisher() *LoggingEventPublisher {
	return &LoggingEventPublisher{}
}

// Publish 发布事件（仅记录日志）
func (p *LoggingEventPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	log.Infof("[LoggingPublisher] event: type=%s, id=%s, aggregate=%s/%s",
		evt.EventType(), evt.EventID(), evt.AggregateType(), evt.AggregateID())
	return nil
}

// PublishAll 批量发布事件
func (p *LoggingEventPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		_ = p.Publish(ctx, evt)
	}
	return nil
}

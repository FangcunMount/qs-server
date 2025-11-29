package event

import (
	"time"

	"github.com/google/uuid"
)

// DomainEvent 领域事件接口
// 这是共享内核（Shared Kernel）的一部分，供所有领域模块使用
//
// 设计原则：
// - 领域事件表达"已发生的业务事实"
// - 事件是不可变的（Immutable）
// - 事件命名使用过去式（如 OrderCreated, PaymentCompleted）
type DomainEvent interface {
	// EventID 事件唯一标识
	// 用于幂等性检查和事件追踪
	EventID() string

	// EventType 事件类型
	// 格式建议：{aggregate}.{action}，如 "assessment.submitted"
	EventType() string

	// OccurredAt 事件发生时间
	OccurredAt() time.Time

	// AggregateType 聚合根类型
	// 如 "Assessment", "Questionnaire", "Testee"
	AggregateType() string

	// AggregateID 聚合根ID
	// 使用 string 类型以支持不同的 ID 格式（int64, UUID 等）
	AggregateID() string
}

// ==================== 事件基类 ====================

// BaseEvent 事件基类
// 提供 DomainEvent 接口的基础实现，具体事件类型可以嵌入此结构
//
// 使用示例：
//
//	type OrderCreatedEvent struct {
//	    event.BaseEvent
//	    OrderID   string
//	    CustomerID string
//	}
type BaseEvent struct {
	id            string
	eventType     string
	occurredAt    time.Time
	aggregateType string
	aggregateID   string
}

// NewBaseEvent 创建事件基类
//
// 参数：
//   - eventType: 事件类型，如 "assessment.submitted"
//   - aggregateType: 聚合根类型，如 "Assessment"
//   - aggregateID: 聚合根ID
func NewBaseEvent(eventType, aggregateType, aggregateID string) BaseEvent {
	return BaseEvent{
		id:            uuid.New().String(),
		eventType:     eventType,
		occurredAt:    time.Now(),
		aggregateType: aggregateType,
		aggregateID:   aggregateID,
	}
}

// EventID 获取事件ID
func (e BaseEvent) EventID() string {
	return e.id
}

// EventType 获取事件类型
func (e BaseEvent) EventType() string {
	return e.eventType
}

// OccurredAt 获取事件发生时间
func (e BaseEvent) OccurredAt() time.Time {
	return e.occurredAt
}

// AggregateType 获取聚合根类型
func (e BaseEvent) AggregateType() string {
	return e.aggregateType
}

// AggregateID 获取聚合根ID
func (e BaseEvent) AggregateID() string {
	return e.aggregateID
}

// ==================== 事件聚合支持 ====================

// EventRaiser 事件产生者接口
// 聚合根可以实现此接口来支持领域事件收集
type EventRaiser interface {
	// Events 获取待发布的领域事件
	Events() []DomainEvent

	// ClearEvents 清空事件列表（通常在事件发布后调用）
	ClearEvents()
}

// EventCollector 事件收集器
// 可嵌入聚合根以提供事件收集功能
type EventCollector struct {
	events []DomainEvent
}

// NewEventCollector 创建事件收集器
func NewEventCollector() *EventCollector {
	return &EventCollector{
		events: make([]DomainEvent, 0),
	}
}

// AddEvent 添加领域事件
func (c *EventCollector) AddEvent(event DomainEvent) {
	c.events = append(c.events, event)
}

// Events 获取所有事件
func (c *EventCollector) Events() []DomainEvent {
	return c.events
}

// ClearEvents 清空事件列表
func (c *EventCollector) ClearEvents() {
	c.events = make([]DomainEvent, 0)
}

// HasEvents 是否有待发布的事件
func (c *EventCollector) HasEvents() bool {
	return len(c.events) > 0
}

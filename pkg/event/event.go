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
	ID                 string    `json:"id"`
	EventTypeValue     string    `json:"eventType"`
	OccurredAtValue    time.Time `json:"occurredAt"`
	AggregateTypeValue string    `json:"aggregateType"`
	AggregateIDValue   string    `json:"aggregateID"`
}

// NewBaseEvent 创建事件基类
//
// 参数：
//   - eventType: 事件类型，如 "assessment.submitted"
//   - aggregateType: 聚合根类型，如 "Assessment"
//   - aggregateID: 聚合根ID
func NewBaseEvent(eventType, aggregateType, aggregateID string) BaseEvent {
	return BaseEvent{
		ID:                 uuid.New().String(),
		EventTypeValue:     eventType,
		OccurredAtValue:    time.Now(),
		AggregateTypeValue: aggregateType,
		AggregateIDValue:   aggregateID,
	}
}

// EventID 获取事件ID
func (e BaseEvent) EventID() string {
	return e.ID
}

// EventType 获取事件类型
func (e BaseEvent) EventType() string {
	return e.EventTypeValue
}

// OccurredAt 获取事件发生时间
func (e BaseEvent) OccurredAt() time.Time {
	return e.OccurredAtValue
}

// AggregateType 获取聚合根类型
func (e BaseEvent) AggregateType() string {
	return e.AggregateTypeValue
}

// AggregateID 获取聚合根ID
func (e BaseEvent) AggregateID() string {
	return e.AggregateIDValue
}

// ==================== 泛型事件 ====================

// Event 泛型领域事件
// 通过泛型参数 T 携带业务数据，减少模板代码
//
// 使用示例：
//
//	// 1. 定义 Payload 数据结构
//	type OrderCreatedData struct {
//	    OrderID    string `json:"order_id"`
//	    CustomerID string `json:"customer_id"`
//	}
//
//	// 2. 定义类型别名（可选，提高可读性）
//	type OrderCreatedEvent = event.Event[OrderCreatedData]
//
//	// 3. 创建事件
//	evt := event.New("order.created", "Order", orderID, OrderCreatedData{...})
type Event[T any] struct {
	BaseEvent
	Data T `json:"data"`
}

// New 创建泛型事件
//
// 参数：
//   - eventType: 事件类型，如 "assessment.submitted"
//   - aggregateType: 聚合根类型，如 "Assessment"
//   - aggregateID: 聚合根ID
//   - data: 业务数据
func New[T any](eventType, aggregateType, aggregateID string, data T) Event[T] {
	return Event[T]{
		BaseEvent: NewBaseEvent(eventType, aggregateType, aggregateID),
		Data:      data,
	}
}

// Payload 获取事件业务数据
func (e Event[T]) Payload() T {
	return e.Data
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

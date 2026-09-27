// Package event owns QS business-event values and host dispatch ports.
// Broker transport, persistence and subscription contracts belong elsewhere.
package event

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DomainEvent is the business fact shared across QS aggregates and workflows.
type DomainEvent interface {
	EventID() string
	EventType() string
	OccurredAt() time.Time
	AggregateType() string
	AggregateID() string
}

// Publisher is the host's domain-event dispatch port, not an MQ producer.
type Publisher interface {
	Publish(context.Context, DomainEvent) error
	PublishAll(context.Context, []DomainEvent) error
}

type EventPublisher = Publisher

type NopEventPublisher struct{}

func NewNopEventPublisher() *NopEventPublisher { return &NopEventPublisher{} }

func (*NopEventPublisher) Publish(context.Context, DomainEvent) error { return nil }

func (*NopEventPublisher) PublishAll(context.Context, []DomainEvent) error { return nil }

// BaseEvent keeps the historical business-event JSON fields and method names.
type BaseEvent struct {
	ID                 string    `json:"id"`
	EventTypeValue     string    `json:"eventType"`
	OccurredAtValue    time.Time `json:"occurredAt"`
	AggregateTypeValue string    `json:"aggregateType"`
	AggregateIDValue   string    `json:"aggregateID"`
}

func NewBaseEvent(eventType, aggregateType, aggregateID string) BaseEvent {
	return BaseEvent{
		ID:                 uuid.New().String(),
		EventTypeValue:     eventType,
		OccurredAtValue:    time.Now(),
		AggregateTypeValue: aggregateType,
		AggregateIDValue:   aggregateID,
	}
}

func (e BaseEvent) EventID() string       { return e.ID }
func (e BaseEvent) EventType() string     { return e.EventTypeValue }
func (e BaseEvent) OccurredAt() time.Time { return e.OccurredAtValue }
func (e BaseEvent) AggregateType() string { return e.AggregateTypeValue }
func (e BaseEvent) AggregateID() string   { return e.AggregateIDValue }

// Event carries QS-owned domain data without depending on a broker type.
type Event[T any] struct {
	BaseEvent
	Data T `json:"data"`
}

func New[T any](eventType, aggregateType, aggregateID string, data T) Event[T] {
	return Event[T]{BaseEvent: NewBaseEvent(eventType, aggregateType, aggregateID), Data: data}
}

func (e Event[T]) Payload() T { return e.Data }

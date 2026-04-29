package event

import base "github.com/FangcunMount/component-base/pkg/event"

type DomainEvent = base.DomainEvent
type Stager = base.Stager
type Publisher = base.Publisher
type EventPublisher = base.EventPublisher
type EventSubscriber = base.EventSubscriber
type EventHandler = base.EventHandler
type EventStore = base.EventStore
type BaseEvent = base.BaseEvent
type Event[T any] = base.Event[T]
type EventRaiser = base.EventRaiser
type EventCollector = base.EventCollector
type NopEventPublisher = base.NopEventPublisher

const SourceDefault = base.SourceDefault

func NewBaseEvent(eventType, aggregateType, aggregateID string) BaseEvent {
	return base.NewBaseEvent(eventType, aggregateType, aggregateID)
}

func New[T any](eventType, aggregateType, aggregateID string, data T) Event[T] {
	return base.New(eventType, aggregateType, aggregateID, data)
}

func NewEventCollector() *EventCollector {
	return base.NewEventCollector()
}

func NewNopEventPublisher() *NopEventPublisher {
	return base.NewNopEventPublisher()
}

package outbox

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	base "github.com/FangcunMount/component-base/pkg/outbox"
)

// ScheduledStager stores durable events that must not be claimed before dueAt.
type ScheduledStager interface {
	StageAt(ctx context.Context, dueAt time.Time, events ...event.DomainEvent) error
}

type PendingEvent = base.PendingEvent
type Store = base.Store
type StatusBucket = base.StatusBucket
type StatusSnapshot = base.StatusSnapshot
type StatusReader = base.StatusReader

// PendingEventRefLister lists pending outbox rows for reconciliation.
type PendingEventRefLister interface {
	ListPendingEventRefs(ctx context.Context, limit int, now time.Time) ([]PendingEventRef, error)
}

type PendingEventRef struct {
	EventID       string
	EventType     string
	NextAttemptAt time.Time
	CreatedAt     time.Time
}
type EventTypeStatusBucket struct {
	EventType       string
	Status          string
	Count           int64
	OldestCreatedAt *time.Time
}

// EventTypeStatusReader exposes per-event-type backlog metrics.
type EventTypeStatusReader interface {
	OutboxStatusByEventType(ctx context.Context, now time.Time) ([]EventTypeStatusBucket, error)
}

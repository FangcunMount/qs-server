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

type PendingEvent struct {
	EventID      string
	Event        event.DomainEvent
	AttemptCount int
}

type Store interface {
	ClaimDueEvents(ctx context.Context, limit int, now time.Time) ([]PendingEvent, error)
	MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error
	MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error
}
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

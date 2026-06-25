package outbox

import (
	"context"
	"time"

	base "github.com/FangcunMount/component-base/pkg/outbox"
)

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

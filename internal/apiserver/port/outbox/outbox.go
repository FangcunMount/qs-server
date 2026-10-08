package outbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	rmoutbox "github.com/FangcunMount/reliable-messaging/outbox"
)

// ScheduledStager stores durable events that must not be claimed before dueAt.
type ScheduledStager interface {
	StageAt(ctx context.Context, dueAt time.Time, events ...event.DomainEvent) error
}

// The public status shape belongs to the SDK.
type StatusBucket = rmoutbox.StatusBucket
type StatusSnapshot = rmoutbox.StatusSnapshot
type StatusReader = rmoutbox.StatusReader

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

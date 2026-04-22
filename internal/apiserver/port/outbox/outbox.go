package outbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// PendingEvent represents a claimed outbox row that is ready to publish.
type PendingEvent struct {
	EventID string
	Event   event.DomainEvent
}

// Store manages due outbox rows.
type Store interface {
	ClaimDueEvents(ctx context.Context, limit int, now time.Time) ([]PendingEvent, error)
	MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error
	MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error
}

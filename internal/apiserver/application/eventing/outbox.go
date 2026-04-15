package eventing

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	defaultOutboxRelayBatchSize  = 50
	defaultOutboxRelayRetryDelay = 10 * time.Second
)

// PendingOutboxEvent represents a claimed outbox row that is ready to publish.
type PendingOutboxEvent struct {
	EventID string
	Event   event.DomainEvent
}

// OutboxStore manages due outbox rows.
type OutboxStore interface {
	ClaimDueEvents(ctx context.Context, limit int, now time.Time) ([]PendingOutboxEvent, error)
	MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error
	MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error
}

// OutboxRelay dispatches due outbox events.
type OutboxRelay interface {
	DispatchDue(ctx context.Context) error
}

type outboxRelay struct {
	name       string
	store      OutboxStore
	publisher  event.EventPublisher
	batchSize  int
	retryDelay time.Duration
}

// NewOutboxRelay creates a generic relay for outbox-backed events.
func NewOutboxRelay(name string, store OutboxStore, publisher event.EventPublisher) OutboxRelay {
	return &outboxRelay{
		name:       name,
		store:      store,
		publisher:  publisher,
		batchSize:  defaultOutboxRelayBatchSize,
		retryDelay: defaultOutboxRelayRetryDelay,
	}
}

func (r *outboxRelay) DispatchDue(ctx context.Context) error {
	if r == nil || r.store == nil || r.publisher == nil {
		return nil
	}

	pendingEvents, err := r.store.ClaimDueEvents(ctx, r.batchSize, time.Now())
	if err != nil {
		return err
	}

	l := logger.L(ctx)
	for _, pending := range pendingEvents {
		if err := r.publisher.Publish(ctx, pending.Event); err != nil {
			l.Warnw("outbox publish failed",
				"relay", r.name,
				"event_id", pending.EventID,
				"event_type", pending.Event.EventType(),
				"error", err.Error(),
			)
			if markErr := r.store.MarkEventFailed(ctx, pending.EventID, err.Error(), time.Now().Add(r.retryDelay)); markErr != nil {
				l.Errorw("outbox mark failed failed",
					"relay", r.name,
					"event_id", pending.EventID,
					"error", markErr.Error(),
				)
			}
			continue
		}

		if err := r.store.MarkEventPublished(ctx, pending.EventID, time.Now()); err != nil {
			l.Errorw("outbox mark published failed",
				"relay", r.name,
				"event_id", pending.EventID,
				"error", err.Error(),
			)
		}
	}

	return nil
}

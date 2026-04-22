package eventing

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	defaultOutboxRelayBatchSize  = 50
	defaultOutboxRelayRetryDelay = 10 * time.Second
)

// PendingOutboxEvent keeps the application-facing alias for the shared outbox contract.
type PendingOutboxEvent = outboxport.PendingEvent

// OutboxStore keeps the application-facing alias for the shared outbox contract.
type OutboxStore = outboxport.Store

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

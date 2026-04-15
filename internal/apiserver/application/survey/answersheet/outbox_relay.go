package answersheet

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	defaultSubmittedEventRelayBatchSize  = 50
	defaultSubmittedEventRelayRetryDelay = 10 * time.Second
)

// SubmittedEventRelay dispatches due answersheet.submitted outbox events.
type SubmittedEventRelay interface {
	DispatchDue(ctx context.Context) error
}

type submittedEventRelay struct {
	store      SubmittedEventOutboxStore
	publisher  event.EventPublisher
	batchSize  int
	retryDelay time.Duration
}

// NewSubmittedEventRelay creates a relay for answersheet.submitted outbox rows.
func NewSubmittedEventRelay(store SubmittedEventOutboxStore, publisher event.EventPublisher) SubmittedEventRelay {
	return &submittedEventRelay{
		store:      store,
		publisher:  publisher,
		batchSize:  defaultSubmittedEventRelayBatchSize,
		retryDelay: defaultSubmittedEventRelayRetryDelay,
	}
}

func (r *submittedEventRelay) DispatchDue(ctx context.Context) error {
	if r == nil || r.store == nil || r.publisher == nil {
		return nil
	}

	events, err := r.store.ClaimDueSubmittedEvents(ctx, r.batchSize, time.Now())
	if err != nil {
		return err
	}

	l := logger.L(ctx)
	for _, pending := range events {
		if err := r.publisher.Publish(ctx, pending.Event); err != nil {
			l.Warnw("answersheet outbox publish failed",
				"event_id", pending.EventID,
				"event_type", pending.Event.EventType(),
				"error", err.Error(),
			)
			if markErr := r.store.MarkSubmittedEventFailed(ctx, pending.EventID, err.Error(), time.Now().Add(r.retryDelay)); markErr != nil {
				l.Errorw("answersheet outbox mark failed failed",
					"event_id", pending.EventID,
					"error", markErr.Error(),
				)
			}
			continue
		}

		if err := r.store.MarkSubmittedEventPublished(ctx, pending.EventID, time.Now()); err != nil {
			l.Errorw("answersheet outbox mark published failed",
				"event_id", pending.EventID,
				"error", err.Error(),
			)
		}
	}

	return nil
}

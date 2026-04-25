package eventing

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
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
	observer   eventobservability.Observer
	batchSize  int
	retryDelay time.Duration
}

// NewOutboxRelay creates a generic relay for outbox-backed events.
func NewOutboxRelay(name string, store OutboxStore, publisher event.EventPublisher) OutboxRelay {
	return NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      name,
		Store:     store,
		Publisher: publisher,
	})
}

type OutboxRelayOptions struct {
	Name       string
	Store      OutboxStore
	Publisher  event.EventPublisher
	Observer   eventobservability.Observer
	BatchSize  int
	RetryDelay time.Duration
}

func NewOutboxRelayWithOptions(opts OutboxRelayOptions) OutboxRelay {
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultOutboxRelayBatchSize
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = defaultOutboxRelayRetryDelay
	}
	if opts.Observer == nil {
		opts.Observer = eventobservability.DefaultObserver()
	}
	return &outboxRelay{
		name:       opts.Name,
		store:      opts.Store,
		publisher:  opts.Publisher,
		observer:   opts.Observer,
		batchSize:  opts.BatchSize,
		retryDelay: opts.RetryDelay,
	}
}

func (r *outboxRelay) DispatchDue(ctx context.Context) error {
	if r == nil || r.store == nil || r.publisher == nil {
		return nil
	}

	pendingEvents, err := r.store.ClaimDueEvents(ctx, r.batchSize, time.Now())
	if err != nil {
		r.observe(ctx, "", "", eventobservability.OutboxOutcomeClaimFailed)
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
				r.observe(ctx, "", pending.Event.EventType(), eventobservability.OutboxOutcomeMarkFailedFailed)
				l.Errorw("outbox mark failed failed",
					"relay", r.name,
					"event_id", pending.EventID,
					"error", markErr.Error(),
				)
			}
			r.observe(ctx, "", pending.Event.EventType(), eventobservability.OutboxOutcomePublishFailed)
			continue
		}

		if err := r.store.MarkEventPublished(ctx, pending.EventID, time.Now()); err != nil {
			r.observe(ctx, "", pending.Event.EventType(), eventobservability.OutboxOutcomeMarkPublishedFailed)
			l.Errorw("outbox mark published failed",
				"relay", r.name,
				"event_id", pending.EventID,
				"error", err.Error(),
			)
			continue
		}
		r.observe(ctx, "", pending.Event.EventType(), eventobservability.OutboxOutcomePublished)
	}

	return nil
}

func (r *outboxRelay) observe(ctx context.Context, topicName, eventType string, outcome eventobservability.OutboxOutcome) {
	if r == nil || r.observer == nil {
		return
	}
	r.observer.ObserveOutbox(ctx, eventobservability.OutboxEvent{
		Relay:     r.name,
		Topic:     topicName,
		EventType: eventType,
		Outcome:   outcome,
	})
}

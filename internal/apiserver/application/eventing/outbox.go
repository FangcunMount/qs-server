package eventing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpriority"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	defaultOutboxRelayBatchSize  = 50
	defaultOutboxRelayRetryDelay = outboxcore.DefaultRelayRetryDelay
	defaultOutboxPublishWorkers  = 8
)

// PendingOutboxEvent keeps the application-facing alias for the shared outbox contract.
type PendingOutboxEvent = outboxport.PendingEvent

// OutboxStore keeps the application-facing alias for the shared outbox contract.
type OutboxStore = outboxport.Store

// OutboxStatusReader keeps the application-facing alias for read-only outbox status.
type OutboxStatusReader = outboxport.StatusReader

type OutboxStatusReporter interface {
	ReportOutboxStatus(ctx context.Context)
}

type OutboxBeforePublishHook interface {
	BeforePublish(ctx context.Context, pending PendingOutboxEvent) error
}

type OutboxBeforePublishFunc func(context.Context, PendingOutboxEvent) error

func (f OutboxBeforePublishFunc) BeforePublish(ctx context.Context, pending PendingOutboxEvent) error {
	if f == nil {
		return nil
	}
	return f(ctx, pending)
}

// OutboxRelay dispatches due outbox events.
type OutboxRelay interface {
	DispatchDue(ctx context.Context) error
}

type outboxRelay struct {
	name           string
	store          OutboxStore
	batchPublisher outboxport.BatchPublisher
	publisher      event.EventPublisher
	observer       eventobservability.Observer
	status         OutboxStatusReporter
	batchSize      int
	publishWorkers int
	retryDelay     time.Duration
	hooks          []OutboxBeforePublishHook
	readyIndex     ReadyIndex
}

type relayPublishResult struct {
	pending   PendingOutboxEvent
	published bool
	err       error
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
	Name                    string
	Store                   OutboxStore
	Publisher               event.EventPublisher
	Observer                eventobservability.Observer
	Status                  OutboxStatusReporter
	BatchSize               int
	RetryDelay              time.Duration
	PublishWorkers          int
	RequireDurablePublisher bool
	BeforePublishHooks      []OutboxBeforePublishHook
	ReadyIndex              ReadyIndex
}

func NewOutboxRelayWithOptions(opts OutboxRelayOptions) OutboxRelay {
	if opts.RequireDurablePublisher && !isDurablePublisher(opts.Publisher) {
		return nil
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultOutboxRelayBatchSize
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = defaultOutboxRelayRetryDelay
	}
	if opts.PublishWorkers <= 0 {
		opts.PublishWorkers = defaultOutboxPublishWorkers
	}
	if opts.Observer == nil {
		opts.Observer = eventobservability.DefaultObserver()
	}
	if opts.Status == nil {
		if reader, ok := opts.Store.(OutboxStatusReader); ok {
			opts.Status = NewOutboxStatusReporter(opts.Name, reader, opts.Observer)
		}
	}
	return &outboxRelay{
		name:           opts.Name,
		store:          opts.Store,
		batchPublisher: batchPublisherOf(opts.Store),
		publisher:      opts.Publisher,
		observer:       opts.Observer,
		status:         opts.Status,
		batchSize:      opts.BatchSize,
		publishWorkers: opts.PublishWorkers,
		retryDelay:     opts.RetryDelay,
		hooks:          compactBeforePublishHooks(opts.BeforePublishHooks),
		readyIndex:     opts.ReadyIndex,
	}
}

func batchPublisherOf(store OutboxStore) outboxport.BatchPublisher {
	if store == nil {
		return nil
	}
	typed, _ := store.(outboxport.BatchPublisher)
	return typed
}

func NewDurableOutboxRelay(name string, store OutboxStore, publisher event.EventPublisher) OutboxRelay {
	return NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:                    name,
		Store:                   store,
		Publisher:               publisher,
		RequireDurablePublisher: true,
	})
}

func NewDurableOutboxRelayWithHooks(name string, store OutboxStore, publisher event.EventPublisher, hooks ...OutboxBeforePublishHook) OutboxRelay {
	return NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:                    name,
		Store:                   store,
		Publisher:               publisher,
		RequireDurablePublisher: true,
		BeforePublishHooks:      hooks,
	})
}

type durablePublisher interface {
	IsMQBacked() bool
}

func isDurablePublisher(publisher event.EventPublisher) bool {
	if publisher == nil {
		return false
	}
	typed, ok := publisher.(durablePublisher)
	return ok && typed.IsMQBacked()
}

func (r *outboxRelay) DispatchDue(ctx context.Context) error {
	if r == nil || r.store == nil || r.publisher == nil {
		return nil
	}
	defer r.reportStatus(ctx)

	now := time.Now()
	pendingEvents, err := r.claimDueEvents(ctx, now)
	if err != nil {
		r.observe(ctx, "", "", eventobservability.OutboxOutcomeClaimFailed)
		return err
	}

	l := logger.L(ctx)

	workers := r.publishWorkers
	if workers <= 0 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	results := make(chan relayPublishResult, len(pendingEvents))
	var wg sync.WaitGroup

	for _, pending := range pendingEvents {
		wg.Add(1)
		go func(item PendingOutboxEvent) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- r.publishOne(ctx, l, item)
		}(pending)
	}
	wg.Wait()
	close(results)

	publishedIDs := make([]string, 0, len(pendingEvents))
	failures := make([]outboxport.FailedMark, 0)
	for result := range results {
		if result.published {
			publishedIDs = append(publishedIDs, result.pending.EventID)
			if r.readyIndex != nil {
				_ = r.readyIndex.Remove(ctx, eventTypeOf(result.pending), result.pending.EventID)
			}
			continue
		}
		if result.err != nil {
			failures = append(failures, outboxport.FailedMark{
				EventID:   result.pending.EventID,
				EventType: eventTypeOf(result.pending),
				LastError: result.err.Error(),
			})
		}
	}

	now = time.Now()
	if r.batchPublisher != nil && len(publishedIDs) > 0 {
		if err := r.batchPublisher.MarkEventsPublished(ctx, publishedIDs, now); err != nil {
			for _, eventID := range publishedIDs {
				r.observe(ctx, "", "", eventobservability.OutboxOutcomeMarkPublishedFailed)
				l.Errorw("outbox batch mark published failed",
					"relay", r.name,
					"event_id", eventID,
					"error", err.Error(),
				)
			}
		} else {
			for _, pending := range pendingEvents {
				for _, eventID := range publishedIDs {
					if pending.EventID == eventID {
						r.observe(ctx, "", pending.Event.EventType(), eventobservability.OutboxOutcomePublished)
					}
				}
			}
		}
	} else {
		for _, pending := range pendingEvents {
			for _, eventID := range publishedIDs {
				if pending.EventID != eventID {
					continue
				}
				if err := r.store.MarkEventPublished(ctx, eventID, now); err != nil {
					r.observe(ctx, "", pending.Event.EventType(), eventobservability.OutboxOutcomeMarkPublishedFailed)
					l.Errorw("outbox mark published failed", "relay", r.name, "event_id", eventID, "error", err.Error())
					continue
				}
				r.observe(ctx, "", pending.Event.EventType(), eventobservability.OutboxOutcomePublished)
			}
		}
	}

	retryAt := now.Add(r.retryDelay)
	if r.batchPublisher != nil && len(failures) > 0 {
		_ = r.batchPublisher.MarkEventsFailed(ctx, failures, retryAt)
		for _, failure := range failures {
			r.observe(ctx, "", "", eventobservability.OutboxOutcomePublishFailed)
			if r.readyIndex != nil {
				_ = r.readyIndex.Enqueue(ctx, failure.EventType, failure.EventID, retryAt)
			}
		}
	} else {
		for _, failure := range failures {
			r.markEventFailed(ctx, l, pendingEventForFailure(failure), fmt.Errorf("%s", failure.LastError))
		}
	}

	return nil
}

func (r *outboxRelay) claimDueEvents(ctx context.Context, now time.Time) ([]PendingOutboxEvent, error) {
	limit := r.batchSize
	claimed := make([]PendingOutboxEvent, 0, limit)

	if r.readyIndex != nil {
		if byIDClaimer, ok := r.store.(outboxport.EventIDClaimer); ok {
			for _, bucket := range outboxpriority.ReadyIndexBuckets {
				if len(claimed) >= limit {
					break
				}
				ids, err := r.readyIndex.ClaimDueIDs(ctx, bucket, limit-len(claimed), now)
				if err != nil {
					logger.L(ctx).Warnw("outbox ready index claim failed",
						"relay", r.name,
						"bucket", bucket,
						"error", err.Error(),
					)
					continue
				}
				if len(ids) == 0 {
					continue
				}
				batch, err := byIDClaimer.ClaimEventsByIDs(ctx, ids, now)
				if err != nil {
					if len(claimed) == 0 {
						return nil, err
					}
					logger.L(ctx).Warnw("outbox claim by ready index ids failed",
						"relay", r.name,
						"bucket", bucket,
						"error", err.Error(),
					)
					break
				}
				claimed = append(claimed, batch...)
				r.pruneUnclaimedReadyIndexIDs(ctx, ids, batch)
			}
		}
		if len(claimed) > 0 {
			return claimed, nil
		}
	}

	fallback, err := r.store.ClaimDueEvents(ctx, limit, now)
	if err != nil {
		return nil, err
	}
	return fallback, nil
}

func (r *outboxRelay) pruneUnclaimedReadyIndexIDs(ctx context.Context, requested []string, claimed []PendingOutboxEvent) {
	if r == nil || r.readyIndex == nil || len(requested) == 0 {
		return
	}
	claimedIDs := make(map[string]struct{}, len(claimed))
	for _, pending := range claimed {
		if pending.EventID != "" {
			claimedIDs[pending.EventID] = struct{}{}
		}
	}
	for _, eventID := range requested {
		if eventID == "" {
			continue
		}
		if _, ok := claimedIDs[eventID]; ok {
			continue
		}
		_ = r.readyIndex.RemoveByEventID(ctx, eventID)
	}
}

func pendingEventForFailure(failure outboxport.FailedMark) PendingOutboxEvent {
	pending := PendingOutboxEvent{EventID: failure.EventID}
	if failure.EventType != "" {
		pending.Event = event.New(failure.EventType, "", failure.EventID, struct{}{})
	}
	return pending
}

func (r *outboxRelay) publishOne(ctx context.Context, l *logger.RequestLogger, pending PendingOutboxEvent) relayPublishResult {
	if err := r.runBeforePublishHooks(ctx, pending); err != nil {
		l.Warnw("outbox before publish hook failed",
			"relay", r.name,
			"event_id", pending.EventID,
			"event_type", eventTypeOf(pending),
			"error", err.Error(),
		)
		return relayPublishResult{pending: pending, err: err}
	}
	if err := r.publisher.Publish(ctx, pending.Event); err != nil {
		l.Warnw("outbox publish failed",
			"relay", r.name,
			"event_id", pending.EventID,
			"event_type", pending.Event.EventType(),
			"error", err.Error(),
		)
		return relayPublishResult{pending: pending, err: err}
	}
	return relayPublishResult{pending: pending, published: true}
}

func compactBeforePublishHooks(hooks []OutboxBeforePublishHook) []OutboxBeforePublishHook {
	if len(hooks) == 0 {
		return nil
	}
	compacted := make([]OutboxBeforePublishHook, 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			compacted = append(compacted, hook)
		}
	}
	return compacted
}

func (r *outboxRelay) runBeforePublishHooks(ctx context.Context, pending PendingOutboxEvent) error {
	for _, hook := range r.hooks {
		if hook == nil {
			continue
		}
		if err := hook.BeforePublish(ctx, pending); err != nil {
			return err
		}
	}
	return nil
}

func (r *outboxRelay) markEventFailed(ctx context.Context, l *logger.RequestLogger, pending PendingOutboxEvent, cause error) {
	eventType := eventTypeOf(pending)
	if markErr := r.store.MarkEventFailed(ctx, pending.EventID, cause.Error(), time.Now().Add(r.retryDelay)); markErr != nil {
		r.observe(ctx, "", eventType, eventobservability.OutboxOutcomeMarkFailedFailed)
		l.Errorw("outbox mark failed failed",
			"relay", r.name,
			"event_id", pending.EventID,
			"error", markErr.Error(),
		)
	}
	r.observe(ctx, "", eventType, eventobservability.OutboxOutcomePublishFailed)
	if r.readyIndex != nil {
		_ = r.readyIndex.Enqueue(ctx, eventType, pending.EventID, time.Now().Add(r.retryDelay))
	}
}

func eventTypeOf(pending PendingOutboxEvent) string {
	if pending.Event == nil {
		return ""
	}
	return pending.Event.EventType()
}

func (r *outboxRelay) reportStatus(ctx context.Context) {
	if r == nil || r.status == nil {
		return
	}
	r.status.ReportOutboxStatus(ctx)
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

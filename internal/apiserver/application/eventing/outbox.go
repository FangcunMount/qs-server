package eventing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
)

const (
	defaultOutboxRelayBatchSize  = 50
	defaultOutboxRelayRetryDelay = outboxcore.DefaultRelayRetryDelay
	defaultOutboxPublishWorkers  = 8
	maxPublishedMarkBatchSize    = 100
)

// PendingOutboxEvent 保留application-facing 别名 用于 共享 outbox 契约。
type PendingOutboxEvent = outboxport.PendingEvent

// OutboxStore 保留application-facing 别名 用于 共享 outbox 契约。
type OutboxStore = outboxport.Store

// OutboxStatusReader 保留application-facing 别名 用于 只读 outbox 状态。
type OutboxStatusReader = outboxport.StatusReader

type OutboxStatusReporter interface {
	ReportOutboxStatus(ctx context.Context)
}

// OutboxRelay 分发due outbox 事件。
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
	readyIndex     ReadyIndex
	readyBuckets   []string
}

type relayPublishResult struct {
	pending   PendingOutboxEvent
	published bool
	err       error
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
	ReadyIndex              ReadyIndex
	ReadyBuckets            []string
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
		readyIndex:     opts.ReadyIndex,
		readyBuckets:   append([]string(nil), opts.ReadyBuckets...),
	}
}

func batchPublisherOf(store OutboxStore) outboxport.BatchPublisher {
	if store == nil {
		return nil
	}
	typed, _ := store.(outboxport.BatchPublisher)
	return typed
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
	results := r.publishDueEvents(ctx, l, pendingEvents)
	failures := make([]outboxport.FailedMark, 0)
	published := newPublishedMarker(ctx, l, r)

	for result := range results {
		if result.published {
			if r.readyIndex != nil {
				_ = r.readyIndex.Remove(ctx, eventTypeOf(result.pending), result.pending.EventID)
			}
			published.add(result.pending)
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
	published.flush()

	now = time.Now()
	retryAt := now.Add(r.retryDelay)
	if r.batchPublisher != nil && len(failures) > 0 {
		_ = r.batchPublisher.MarkEventsFailed(ctx, failures, retryAt)
		for _, failure := range failures {
			r.observe(ctx, "", "", eventobservability.OutboxOutcomePublishFailed)
			if r.readyIndex != nil {
				_ = r.readyIndex.Enqueue(ctx, failure.EventType, failure.EventID, retryAt, retryAt)
			}
		}
	} else {
		for _, failure := range failures {
			r.markEventFailed(ctx, l, pendingEventForFailure(failure), fmt.Errorf("%s", failure.LastError))
		}
	}

	return nil
}

func (r *outboxRelay) publishDueEvents(ctx context.Context, l *logger.RequestLogger, pendingEvents []PendingOutboxEvent) <-chan relayPublishResult {
	workers := r.publishWorkers
	if workers <= 0 {
		workers = 1
	}
	results := make(chan relayPublishResult, len(pendingEvents))
	if len(pendingEvents) == 0 {
		close(results)
		return results
	}

	jobs := make(chan PendingOutboxEvent, len(pendingEvents))
	for _, pending := range pendingEvents {
		jobs <- pending
	}
	close(jobs)

	workerCount := workers
	if workerCount > len(pendingEvents) {
		workerCount = len(pendingEvents)
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				results <- r.publishOne(ctx, l, item)
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

type publishedMarker struct {
	ctx     context.Context
	log     *logger.RequestLogger
	relay   *outboxRelay
	pending []PendingOutboxEvent
	limit   int
}

func newPublishedMarker(ctx context.Context, l *logger.RequestLogger, relay *outboxRelay) *publishedMarker {
	limit := relay.publishWorkers
	if limit <= 0 {
		limit = 1
	}
	if limit > maxPublishedMarkBatchSize {
		limit = maxPublishedMarkBatchSize
	}
	return &publishedMarker{
		ctx:   ctx,
		log:   l,
		relay: relay,
		limit: limit,
	}
}

func (m *publishedMarker) add(pending PendingOutboxEvent) {
	m.pending = append(m.pending, pending)
	if len(m.pending) >= m.limit {
		m.flush()
	}
}

func (m *publishedMarker) flush() {
	if m == nil || len(m.pending) == 0 {
		return
	}
	batch := m.pending
	m.pending = nil
	now := time.Now()
	if m.relay.batchPublisher != nil {
		m.markBatch(batch, now)
		return
	}
	m.markOneByOne(batch, now)
}

func (m *publishedMarker) markBatch(batch []PendingOutboxEvent, now time.Time) {
	eventIDs := make([]string, 0, len(batch))
	for _, pending := range batch {
		eventIDs = append(eventIDs, pending.EventID)
	}
	if err := m.relay.batchPublisher.MarkEventsPublished(m.ctx, eventIDs, now); err != nil {
		for _, pending := range batch {
			m.relay.observe(m.ctx, "", eventTypeOf(pending), eventobservability.OutboxOutcomeMarkPublishedFailed)
			m.log.Errorw("outbox batch mark published failed",
				"relay", m.relay.name,
				"event_id", pending.EventID,
				"error", err.Error(),
			)
		}
		return
	}
	for _, pending := range batch {
		m.relay.observe(m.ctx, "", eventTypeOf(pending), eventobservability.OutboxOutcomePublished)
	}
}

func (m *publishedMarker) markOneByOne(batch []PendingOutboxEvent, now time.Time) {
	for _, pending := range batch {
		if err := m.relay.store.MarkEventPublished(m.ctx, pending.EventID, now); err != nil {
			m.relay.observe(m.ctx, "", eventTypeOf(pending), eventobservability.OutboxOutcomeMarkPublishedFailed)
			m.log.Errorw("outbox mark published failed", "relay", m.relay.name, "event_id", pending.EventID, "error", err.Error())
			continue
		}
		m.relay.observe(m.ctx, "", eventTypeOf(pending), eventobservability.OutboxOutcomePublished)
	}
}

func (r *outboxRelay) claimDueEvents(ctx context.Context, now time.Time) ([]PendingOutboxEvent, error) {
	limit := r.batchSize
	claimed := make([]PendingOutboxEvent, 0, limit)

	if r.readyIndex != nil {
		if byIDClaimer, ok := r.store.(outboxport.EventIDClaimer); ok {
			for _, bucket := range r.readyBuckets {
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

func pendingEventForFailure(failure outboxport.FailedMark) PendingOutboxEvent {
	pending := PendingOutboxEvent{EventID: failure.EventID}
	if failure.EventType != "" {
		pending.Event = event.New(failure.EventType, "", failure.EventID, struct{}{})
	}
	return pending
}

func (r *outboxRelay) publishOne(ctx context.Context, l *logger.RequestLogger, pending PendingOutboxEvent) relayPublishResult {
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
		createdAt := time.Now()
		if pending.Event != nil && !pending.Event.OccurredAt().IsZero() {
			createdAt = pending.Event.OccurredAt()
		}
		_ = r.readyIndex.Enqueue(ctx, eventType, pending.EventID, time.Now().Add(r.retryDelay), createdAt)
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

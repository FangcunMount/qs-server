package eventing

import (
	"context"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/logger"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
)

const (
	defaultImmediateDispatchTimeout = 2 * time.Second
	defaultImmediateMaxConcurrent   = 16
)

// ImmediateDispatcher best-effort 发布 暂存的 outbox 事件 事务提交后立即。
type ImmediateDispatcher struct {
	name                string
	store               OutboxStore
	reader              outboxport.ImmediatePublishReader
	publisher           event.EventPublisher
	observer            eventobservability.Observer
	enabled             bool
	timeout             time.Duration
	sem                 chan struct{}
	readyIndex          ReadyIndex
	readyIndexer        *PostCommitReadyIndexer
	immediateEventTypes map[string]struct{}
	lifecycleMu         sync.Mutex
	closed              bool
	wg                  sync.WaitGroup
}

type ImmediateDispatcherOptions struct {
	Name      string
	Store     OutboxStore
	Publisher event.EventPublisher
	Observer  eventobservability.Observer
	Enabled   bool
	// RequireDurablePublisher prevents a durable outbox event from being
	// acknowledged as published by logging/nop publisher modes.
	RequireDurablePublisher bool
	Timeout                 time.Duration
	MaxConcurrent           int
	ReadyIndex              ReadyIndex
	ImmediateEventTypes     []string
}

func NewImmediateDispatcher(opts ImmediateDispatcherOptions) *ImmediateDispatcher {
	reader, _ := opts.Store.(outboxport.ImmediatePublishReader)
	if opts.Timeout <= 0 {
		opts.Timeout = defaultImmediateDispatchTimeout
	}
	if opts.Observer == nil {
		opts.Observer = eventobservability.DefaultObserver()
	}
	maxConcurrent := opts.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = defaultImmediateMaxConcurrent
	}
	enabled := opts.Enabled && reader != nil && opts.Publisher != nil
	if opts.RequireDurablePublisher && !isDurablePublisher(opts.Publisher) {
		enabled = false
	}
	return &ImmediateDispatcher{
		name:                opts.Name,
		store:               opts.Store,
		reader:              reader,
		publisher:           opts.Publisher,
		observer:            opts.Observer,
		enabled:             enabled,
		timeout:             opts.Timeout,
		sem:                 make(chan struct{}, maxConcurrent),
		readyIndex:          opts.ReadyIndex,
		readyIndexer:        NewPostCommitReadyIndexer(opts.ReadyIndex),
		immediateEventTypes: eventTypeSet(opts.ImmediateEventTypes),
	}
}

func (d *ImmediateDispatcher) TryDispatchAfterCommit(ctx context.Context, events []event.DomainEvent) {
	d.AfterCommit(ctx, events, time.Now())
}

// AfterCommit records ready-index hints and attempts immediate delivery for
// explicitly eligible event types. It must only be called after commit.
func (d *ImmediateDispatcher) AfterCommit(ctx context.Context, events []event.DomainEvent, readyAt time.Time) {
	if d == nil || len(events) == 0 {
		return
	}
	if readyAt.IsZero() {
		readyAt = time.Now()
	}
	if d.readyIndexer != nil {
		d.readyIndexer.EnqueueAfterCommit(ctx, events, readyAt)
	}
	if !d.enabled {
		return
	}
	for _, evt := range events {
		if evt == nil {
			continue
		}
		if _, ok := d.immediateEventTypes[evt.EventType()]; !ok {
			continue
		}
		eventID := evt.EventID()
		eventType := evt.EventType()
		if !d.reserveDispatch() {
			return
		}
		go func() {
			defer d.wg.Done()
			select {
			case d.sem <- struct{}{}:
				defer func() { <-d.sem }()
			default:
				d.observeImmediate(ctx, eventType, "immediate_skipped")
				return
			}
			ctx, cancel := detachedContext(ctx, d.timeout)
			defer cancel()
			d.dispatchOne(ctx, eventID, eventType)
		}()
	}
}

func (d *ImmediateDispatcher) reserveDispatch() bool {
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	if d.closed {
		return false
	}
	d.wg.Add(1)
	return true
}

// Close prevents new immediate attempts and waits for attempts already owned
// by this dispatcher. It does not close the injected store or publisher.
func (d *ImmediateDispatcher) Close() {
	if d == nil {
		return
	}
	d.lifecycleMu.Lock()
	d.closed = true
	d.lifecycleMu.Unlock()
	d.wg.Wait()
}

func detachedContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

func (d *ImmediateDispatcher) dispatchOne(ctx context.Context, eventID, eventType string) {
	now := time.Now()
	pending, found, err := d.reader.GetPublishableEvent(ctx, eventID, now)
	if err != nil || !found {
		d.observeImmediate(ctx, eventType, "not_found")
		return
	}
	l := logger.L(ctx)
	if err := d.publisher.Publish(ctx, pending.Event); err != nil {
		d.observeImmediate(ctx, eventType, "publish_failed")
		l.Warnw("immediate outbox publish failed",
			"dispatcher", d.name,
			"event_id", eventID,
			"error", err.Error(),
		)
		return
	}
	if err := d.store.MarkEventPublished(ctx, eventID, now); err != nil {
		d.observeImmediate(ctx, eventType, "mark_failed")
		l.Warnw("immediate outbox mark published failed",
			"dispatcher", d.name,
			"event_id", eventID,
			"error", err.Error(),
		)
		return
	}
	if d.readyIndex != nil {
		_ = d.readyIndex.Remove(ctx, eventType, eventID)
	}
	d.observeImmediate(ctx, eventType, "published")
}

func (d *ImmediateDispatcher) observeImmediate(ctx context.Context, eventType, outcome string) {
	if d == nil || d.observer == nil {
		return
	}
	mapped := eventobservability.OutboxOutcomePublished
	switch outcome {
	case "publish_failed":
		mapped = eventobservability.OutboxOutcomePublishFailed
	case "mark_failed":
		mapped = eventobservability.OutboxOutcomeMarkPublishedFailed
	case "not_found":
		mapped = eventobservability.OutboxOutcomeClaimFailed
	case "immediate_skipped":
		mapped = eventobservability.OutboxOutcomeImmediateSkipped
	}
	d.observer.ObserveOutbox(ctx, eventobservability.OutboxEvent{
		Relay:     d.name + ":immediate",
		EventType: eventType,
		Outcome:   mapped,
	})
}

func eventTypeSet(eventTypes []string) map[string]struct{} {
	set := make(map[string]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		set[eventType] = struct{}{}
	}
	return set
}

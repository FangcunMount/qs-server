package eventing

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	defaultImmediateDispatchTimeout = 2 * time.Second
	defaultImmediateMaxConcurrent   = 16
)

// immediateDispatchEventTypes 走 post-commit immediate 旁路。
// answersheet.submitted：Mongo 主链路；evaluation.requested：MySQL assessment outbox（Mongo immediate 查不到则 noop）。
var immediateDispatchEventTypes = map[string]struct{}{
	eventcatalog.AnswerSheetSubmitted:       {},
	eventcatalog.EvaluationRequested:        {},
	eventcatalog.EvaluationOutcomeCommitted: {},
}

// ImmediateDispatcher best-effort 发布 暂存的 outbox 事件 事务提交后立即。
type ImmediateDispatcher struct {
	name         string
	store        OutboxStore
	reader       outboxport.ImmediatePublishReader
	publisher    event.EventPublisher
	observer     eventobservability.Observer
	enabled      bool
	timeout      time.Duration
	sem          chan struct{}
	hooks        []OutboxBeforePublishHook
	readyIndex   ReadyIndex
	readyIndexer *PostCommitReadyIndexer
}

type ImmediateDispatcherOptions struct {
	Name          string
	Store         OutboxStore
	Publisher     event.EventPublisher
	Observer      eventobservability.Observer
	Enabled       bool
	Timeout       time.Duration
	MaxConcurrent int
	Hooks         []OutboxBeforePublishHook
	ReadyIndex    ReadyIndex
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
	return &ImmediateDispatcher{
		name:         opts.Name,
		store:        opts.Store,
		reader:       reader,
		publisher:    opts.Publisher,
		observer:     opts.Observer,
		enabled:      opts.Enabled && reader != nil && opts.Publisher != nil,
		timeout:      opts.Timeout,
		sem:          make(chan struct{}, maxConcurrent),
		hooks:        compactBeforePublishHooks(opts.Hooks),
		readyIndex:   opts.ReadyIndex,
		readyIndexer: NewPostCommitReadyIndexer(opts.ReadyIndex),
	}
}

func (d *ImmediateDispatcher) TryDispatchAfterCommit(ctx context.Context, events []event.DomainEvent) {
	if d == nil || len(events) == 0 {
		return
	}
	if d.readyIndexer != nil {
		d.readyIndexer.EnqueueAfterCommit(ctx, events, time.Now())
	}
	if !d.enabled {
		return
	}
	for _, evt := range events {
		if evt == nil {
			continue
		}
		if _, ok := immediateDispatchEventTypes[evt.EventType()]; !ok {
			continue
		}
		eventID := evt.EventID()
		eventType := evt.EventType()
		go func() {
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
	for _, hook := range d.hooks {
		if hook == nil {
			continue
		}
		if err := hook.BeforePublish(ctx, pending); err != nil {
			d.observeImmediate(ctx, eventType, "hook_failed")
			l.Warnw("immediate outbox hook failed",
				"dispatcher", d.name,
				"event_id", eventID,
				"error", err.Error(),
			)
			return
		}
	}
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
	case "hook_failed", "not_found":
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

func IsImmediateDispatchEventType(eventType string) bool {
	_, ok := immediateDispatchEventTypes[eventType]
	return ok
}

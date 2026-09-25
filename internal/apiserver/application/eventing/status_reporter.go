package eventing

import (
	"context"
	"sort"
	"sync"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
)

const defaultOutboxStatusReportInterval = 30 * time.Second

type outboxStatusReporter struct {
	name          string
	reader        outboxport.StatusReader
	observer      eventobservability.Observer
	eventTypes    []string
	typeSeen      map[eventTypeStatusKey]struct{}
	now           func() time.Time
	minInterval   time.Duration
	mu            sync.Mutex
	lastAttemptAt time.Time
	inFlight      bool
}

type eventTypeStatusKey struct {
	eventType string
	status    string
}

// NewOutboxStatusReporter 创建best-effort 指标桥接器 用于 一个outbox 存储。
func NewOutboxStatusReporter(name string, reader outboxport.StatusReader, observer eventobservability.Observer) OutboxStatusReporter {
	return newOutboxStatusReporter(name, reader, observer, time.Now)
}

// NewOutboxStatusReporterWithEventTypes seeds zero-valued series for a standard
// profile, so an event type with no backlog is observable from the first scrape.
func NewOutboxStatusReporterWithEventTypes(name string, reader outboxport.StatusReader, observer eventobservability.Observer, eventTypes []string) OutboxStatusReporter {
	reporter := newOutboxStatusReporter(name, reader, observer, time.Now)
	reporter.eventTypes = append([]string(nil), eventTypes...)
	return reporter
}

func newOutboxStatusReporter(name string, reader outboxport.StatusReader, observer eventobservability.Observer, now func() time.Time) *outboxStatusReporter {
	return newOutboxStatusReporterWithInterval(name, reader, observer, now, defaultOutboxStatusReportInterval)
}

func newOutboxStatusReporterWithInterval(name string, reader outboxport.StatusReader, observer eventobservability.Observer, now func() time.Time, minInterval time.Duration) *outboxStatusReporter {
	if now == nil {
		now = time.Now
	}
	if observer == nil {
		observer = eventobservability.DefaultObserver()
	}
	return &outboxStatusReporter{name: name, reader: reader, observer: observer, now: now, minInterval: minInterval}
}

func (r *outboxStatusReporter) ReportOutboxStatus(ctx context.Context) {
	if r == nil || r.reader == nil {
		return
	}
	now := r.now()
	r.mu.Lock()
	if r.inFlight || (r.minInterval > 0 && !r.lastAttemptAt.IsZero() && now.Sub(r.lastAttemptAt) < r.minInterval) {
		r.mu.Unlock()
		return
	}
	r.lastAttemptAt = now
	r.inFlight = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.inFlight = false
		r.mu.Unlock()
	}()

	storeName := r.name
	snapshot, err := r.reader.OutboxStatusSnapshot(ctx, now)
	if snapshot.Store != "" {
		storeName = snapshot.Store
	}
	if err != nil {
		eventobservability.ObserveOutboxStatusScrape(ctx, r.observer, eventobservability.OutboxStatusScrapeEvent{
			Store:   storeName,
			Outcome: eventobservability.OutboxStatusScrapeOutcomeFailure,
		})
		return
	}
	reportOutboxStatusSnapshot(ctx, r.observer, storeName, snapshot)
	if typeReader, ok := r.reader.(outboxport.EventTypeStatusReader); ok {
		if err := r.reportOutboxEventTypeStatus(ctx, storeName, typeReader, snapshot.Buckets, now); err != nil {
			eventobservability.ObserveOutboxStatusScrape(ctx, r.observer, eventobservability.OutboxStatusScrapeEvent{
				Store: storeName, Outcome: eventobservability.OutboxStatusScrapeOutcomeFailure,
			})
			return
		}
	}
	eventobservability.ObserveOutboxStatusScrape(ctx, r.observer, eventobservability.OutboxStatusScrapeEvent{
		Store: storeName, Outcome: eventobservability.OutboxStatusScrapeOutcomeSuccess,
	})
}

func reportOutboxStatusSnapshot(ctx context.Context, observer eventobservability.Observer, storeName string, snapshot outboxport.StatusSnapshot) {
	if snapshot.Store != "" {
		storeName = snapshot.Store
	}
	for _, bucket := range snapshot.Buckets {
		eventobservability.ObserveOutboxStatus(ctx, observer, eventobservability.OutboxStatusEvent{
			Store:            storeName,
			Status:           bucket.Status,
			Count:            bucket.Count,
			OldestAgeSeconds: bucket.OldestAgeSeconds,
		})
	}
}

func (r *outboxStatusReporter) reportOutboxEventTypeStatus(ctx context.Context, storeName string, reader outboxport.EventTypeStatusReader, states []outboxport.StatusBucket, now time.Time) error {
	buckets, err := reader.OutboxStatusByEventType(ctx, now)
	if err != nil {
		return err
	}
	current := make(map[eventTypeStatusKey]eventobservability.OutboxEventTypeStatusEvent, len(buckets))
	all := make(map[eventTypeStatusKey]struct{}, len(r.typeSeen)+len(buckets)+len(r.eventTypes)*len(states))
	for key := range r.typeSeen {
		all[key] = struct{}{}
	}
	for _, eventType := range r.eventTypes {
		for _, state := range states {
			all[eventTypeStatusKey{eventType: eventType, status: state.Status}] = struct{}{}
		}
	}
	for _, bucket := range buckets {
		age := 0.0
		if bucket.OldestCreatedAt != nil {
			age = now.Sub(*bucket.OldestCreatedAt).Seconds()
			if age < 0 {
				age = 0
			}
		}
		key := eventTypeStatusKey{eventType: bucket.EventType, status: bucket.Status}
		all[key] = struct{}{}
		current[key] = eventobservability.OutboxEventTypeStatusEvent{
			Store:            storeName,
			EventType:        bucket.EventType,
			Status:           bucket.Status,
			Count:            bucket.Count,
			OldestAgeSeconds: age,
		}
	}
	ordered := make([]eventTypeStatusKey, 0, len(all))
	for key := range all {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].eventType != ordered[j].eventType {
			return ordered[i].eventType < ordered[j].eventType
		}
		return ordered[i].status < ordered[j].status
	})
	for _, key := range ordered {
		evt, found := current[key]
		if !found {
			evt = eventobservability.OutboxEventTypeStatusEvent{Store: storeName, EventType: key.eventType, Status: key.status}
		}
		eventobservability.ObserveOutboxEventTypeStatus(ctx, r.observer, evt)
	}
	r.typeSeen = all
	return nil
}

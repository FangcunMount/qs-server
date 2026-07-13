package eventing

import (
	"context"
	"sync"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
)

const defaultOutboxStatusReportInterval = 30 * time.Second

type outboxStatusReporter struct {
	name          string
	reader        OutboxStatusReader
	observer      eventobservability.Observer
	now           func() time.Time
	minInterval   time.Duration
	mu            sync.Mutex
	lastAttemptAt time.Time
}

// NewOutboxStatusReporter 创建best-effort 指标桥接器 用于 一个outbox 存储。
func NewOutboxStatusReporter(name string, reader OutboxStatusReader, observer eventobservability.Observer) OutboxStatusReporter {
	return newOutboxStatusReporter(name, reader, observer, time.Now)
}

func newOutboxStatusReporter(name string, reader OutboxStatusReader, observer eventobservability.Observer, now func() time.Time) OutboxStatusReporter {
	return newOutboxStatusReporterWithInterval(name, reader, observer, now, defaultOutboxStatusReportInterval)
}

func newOutboxStatusReporterWithInterval(name string, reader OutboxStatusReader, observer eventobservability.Observer, now func() time.Time, minInterval time.Duration) OutboxStatusReporter {
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
	if r.minInterval > 0 && !r.lastAttemptAt.IsZero() && now.Sub(r.lastAttemptAt) < r.minInterval {
		r.mu.Unlock()
		return
	}
	r.lastAttemptAt = now
	r.mu.Unlock()

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
		reportOutboxEventTypeStatus(ctx, r.observer, storeName, typeReader, now)
	}
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
	eventobservability.ObserveOutboxStatusScrape(ctx, observer, eventobservability.OutboxStatusScrapeEvent{
		Store:   storeName,
		Outcome: eventobservability.OutboxStatusScrapeOutcomeSuccess,
	})
}

func reportOutboxEventTypeStatus(ctx context.Context, observer eventobservability.Observer, storeName string, reader outboxport.EventTypeStatusReader, now time.Time) {
	buckets, err := reader.OutboxStatusByEventType(ctx, now)
	if err != nil {
		return
	}
	for _, bucket := range buckets {
		age := 0.0
		if bucket.OldestCreatedAt != nil {
			age = now.Sub(*bucket.OldestCreatedAt).Seconds()
		}
		eventobservability.ObserveOutboxEventTypeStatus(ctx, observer, eventobservability.OutboxEventTypeStatusEvent{
			Store:            storeName,
			EventType:        bucket.EventType,
			Status:           bucket.Status,
			Count:            bucket.Count,
			OldestAgeSeconds: age,
		})
	}
}

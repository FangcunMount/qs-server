package eventing

import (
	"context"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
)

type outboxStatusReporter struct {
	name     string
	reader   OutboxStatusReader
	observer eventobservability.Observer
	now      func() time.Time
}

// NewOutboxStatusReporter creates the best-effort metrics bridge for one outbox store.
func NewOutboxStatusReporter(name string, reader OutboxStatusReader, observer eventobservability.Observer) OutboxStatusReporter {
	return newOutboxStatusReporter(name, reader, observer, time.Now)
}

func newOutboxStatusReporter(name string, reader OutboxStatusReader, observer eventobservability.Observer, now func() time.Time) OutboxStatusReporter {
	if now == nil {
		now = time.Now
	}
	if observer == nil {
		observer = eventobservability.DefaultObserver()
	}
	return &outboxStatusReporter{name: name, reader: reader, observer: observer, now: now}
}

func (r *outboxStatusReporter) ReportOutboxStatus(ctx context.Context) {
	if r == nil || r.reader == nil {
		return
	}
	storeName := r.name
	snapshot, err := r.reader.OutboxStatusSnapshot(ctx, r.now())
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

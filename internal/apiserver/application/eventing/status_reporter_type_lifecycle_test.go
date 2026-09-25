package eventing

import (
	"context"
	"errors"
	"testing"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
)

type typeLifecycleReader struct {
	rows []outboxport.EventTypeStatusBucket
	err  error
}

func (*typeLifecycleReader) OutboxStatusSnapshot(_ context.Context, now time.Time) (outboxport.StatusSnapshot, error) {
	return outboxport.StatusSnapshot{Store: "mongo-domain-events", GeneratedAt: now, Buckets: []outboxport.StatusBucket{
		{Status: "pending"}, {Status: "retry_wait"}, {Status: "publishing"}, {Status: "quarantined"},
	}}, nil
}

func (r *typeLifecycleReader) OutboxStatusByEventType(context.Context, time.Time) ([]outboxport.EventTypeStatusBucket, error) {
	return r.rows, r.err
}

type typeLifecycleObserver struct {
	eventobservability.NopObserver
	byType  []eventobservability.OutboxEventTypeStatusEvent
	scrapes []eventobservability.OutboxStatusScrapeEvent
}

func (o *typeLifecycleObserver) ObserveOutboxEventTypeStatus(_ context.Context, evt eventobservability.OutboxEventTypeStatusEvent) {
	o.byType = append(o.byType, evt)
}

func (o *typeLifecycleObserver) ObserveOutboxStatusScrape(_ context.Context, evt eventobservability.OutboxStatusScrapeEvent) {
	o.scrapes = append(o.scrapes, evt)
}

func TestOutboxTypeStatusSeedsAndClearsLabelsOnlyAfterSuccessfulRead(t *testing.T) {
	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	oldest := now.Add(-time.Minute)
	reader := &typeLifecycleReader{rows: []outboxport.EventTypeStatusBucket{
		{EventType: "answersheet.submitted", Status: "pending", Count: 2, OldestCreatedAt: &oldest},
		{EventType: "historic.event", Status: "retry_wait", Count: 3, OldestCreatedAt: &oldest},
	}}
	observer := &typeLifecycleObserver{}
	reporter := newOutboxStatusReporterWithInterval("mongo-domain-events", reader, observer, func() time.Time { return now }, 0)
	reporter.eventTypes = []string{"answersheet.submitted", "interpretation.report.failed"}

	reporter.ReportOutboxStatus(t.Context())
	first := typeStatusMap(t, observer.byType)
	if len(first) != 9 || first[eventTypeStatusKey{"answersheet.submitted", "pending"}].Count != 2 ||
		first[eventTypeStatusKey{"interpretation.report.failed", "pending"}].Count != 0 ||
		first[eventTypeStatusKey{"historic.event", "retry_wait"}].Count != 3 {
		t.Fatalf("first type snapshot = %#v; want all known zero labels and two live buckets", first)
	}
	if first[eventTypeStatusKey{"answersheet.submitted", "pending"}].OldestAgeSeconds != 60 {
		t.Fatalf("first pending age = %v, want 60", first[eventTypeStatusKey{"answersheet.submitted", "pending"}].OldestAgeSeconds)
	}

	reader.err = errors.New("controlled type aggregation failure")
	beforeFailure := len(observer.byType)
	reporter.ReportOutboxStatus(t.Context())
	if len(observer.byType) != beforeFailure || len(observer.scrapes) != 2 ||
		observer.scrapes[1].Outcome != eventobservability.OutboxStatusScrapeOutcomeFailure {
		t.Fatalf("failed type read must not replace known values: events=%d scrapes=%#v", len(observer.byType), observer.scrapes)
	}

	reader.err = nil
	reader.rows = nil
	reporter.ReportOutboxStatus(t.Context())
	cleared := typeStatusMap(t, observer.byType[beforeFailure:])
	if len(cleared) != 9 {
		t.Fatalf("cleared labels = %d, want 9", len(cleared))
	}
	for key, evt := range cleared {
		if evt.Count != 0 || evt.OldestAgeSeconds != 0 {
			t.Fatalf("stale label %v = %+v, want zero", key, evt)
		}
	}
	if len(observer.scrapes) != 3 || observer.scrapes[2].Outcome != eventobservability.OutboxStatusScrapeOutcomeSuccess {
		t.Fatalf("successful zero snapshot was not recorded: %#v", observer.scrapes)
	}
}

func typeStatusMap(t *testing.T, events []eventobservability.OutboxEventTypeStatusEvent) map[eventTypeStatusKey]eventobservability.OutboxEventTypeStatusEvent {
	t.Helper()
	byKey := make(map[eventTypeStatusKey]eventobservability.OutboxEventTypeStatusEvent, len(events))
	for _, evt := range events {
		key := eventTypeStatusKey{eventType: evt.EventType, status: evt.Status}
		if _, exists := byKey[key]; exists {
			t.Fatalf("duplicate type status event %v", key)
		}
		byKey[key] = evt
	}
	return byKey
}

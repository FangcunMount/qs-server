package outboxcore

import (
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type fakeResolver struct {
	topics     map[string]string
	deliveries map[string]eventcatalog.DeliveryClass
}

func (r fakeResolver) GetTopicForEvent(eventType string) (string, bool) {
	topic, ok := r.topics[eventType]
	return topic, ok
}

func (r fakeResolver) GetDeliveryClass(eventType string) (eventcatalog.DeliveryClass, bool) {
	delivery, ok := r.deliveries[eventType]
	return delivery, ok
}

func TestBuildRecordsBuildsPendingDurableRecords(t *testing.T) {
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	evt := event.New("sample.created", "Sample", "sample-1", map[string]string{"id": "sample-1"})

	records, err := BuildRecords(BuildRecordsOptions{
		Events: []event.DomainEvent{evt},
		Resolver: fakeResolver{
			topics:     map[string]string{"sample.created": "sample.topic"},
			deliveries: map[string]eventcatalog.DeliveryClass{"sample.created": eventcatalog.DeliveryClassDurableOutbox},
		},
		Now: now,
	})
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	record := records[0]
	if record.EventID != evt.EventID() || record.EventType != evt.EventType() || record.AggregateType != evt.AggregateType() || record.AggregateID != evt.AggregateID() {
		t.Fatalf("record identity = %#v, want event identity", record)
	}
	if record.TopicName != "sample.topic" {
		t.Fatalf("topic = %q, want sample.topic", record.TopicName)
	}
	if record.Status != StatusPending || record.AttemptCount != 0 {
		t.Fatalf("state = %q/%d, want pending/0", record.Status, record.AttemptCount)
	}
	if !record.NextAttemptAt.Equal(now) || !record.CreatedAt.Equal(now) || !record.UpdatedAt.Equal(now) {
		t.Fatalf("record times = %#v, want %s", record, now)
	}
	pending, err := DecodePendingEvent(record.EventID, record.PayloadJSON)
	if err != nil {
		t.Fatalf("DecodePendingEvent: %v", err)
	}
	if pending.Event.EventType() != evt.EventType() || pending.Event.AggregateID() != evt.AggregateID() {
		t.Fatalf("decoded event = %#v, want %q/%q", pending.Event, evt.EventType(), evt.AggregateID())
	}
}

func TestBuildRecordsRejectsUnknownEvent(t *testing.T) {
	evt := event.New("sample.created", "Sample", "sample-1", map[string]string{})

	_, err := BuildRecords(BuildRecordsOptions{
		Events:   []event.DomainEvent{evt},
		Resolver: fakeResolver{},
	})
	if err == nil {
		t.Fatalf("BuildRecords should reject unknown event")
	}
	if !strings.Contains(err.Error(), "sample.created") {
		t.Fatalf("error = %v, want event type", err)
	}
}

func TestBuildRecordsRejectsBestEffortEvent(t *testing.T) {
	evt := event.New("sample.changed", "Sample", "sample-1", map[string]string{})

	_, err := BuildRecords(BuildRecordsOptions{
		Events: []event.DomainEvent{evt},
		Resolver: fakeResolver{
			topics:     map[string]string{"sample.changed": "sample.topic"},
			deliveries: map[string]eventcatalog.DeliveryClass{"sample.changed": eventcatalog.DeliveryClassBestEffort},
		},
	})
	if err == nil {
		t.Fatalf("BuildRecords should reject best-effort event")
	}
	if !strings.Contains(err.Error(), "best_effort") {
		t.Fatalf("error = %v, want delivery class", err)
	}
}

func TestDecodePendingEventRejectsInvalidPayload(t *testing.T) {
	_, err := DecodePendingEvent("evt-1", "{not-json")
	if err == nil {
		t.Fatalf("DecodePendingEvent should reject invalid payload")
	}
}

func TestTransitions(t *testing.T) {
	now := time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)
	published := NewPublishedTransition(now)
	if published.Status != StatusPublished || !published.PublishedAt.Equal(now) || !published.UpdatedAt.Equal(now) {
		t.Fatalf("published transition = %#v", published)
	}

	next := now.Add(time.Minute)
	failed := NewFailedTransition("boom", next, now)
	if failed.Status != StatusFailed || failed.LastError != "boom" || !failed.NextAttemptAt.Equal(next) || !failed.UpdatedAt.Equal(now) || failed.AttemptIncrement != 1 {
		t.Fatalf("failed transition = %#v", failed)
	}

	decode := NewDecodeFailureTransition(assertErr("bad payload"), now)
	if decode.Status != StatusFailed || !strings.Contains(decode.LastError, "decode outbox payload: bad payload") {
		t.Fatalf("decode failure transition = %#v", decode)
	}
	if !decode.NextAttemptAt.Equal(now.Add(DefaultDecodeFailureRetryDelay)) || !decode.UpdatedAt.Equal(now) {
		t.Fatalf("decode failure times = %#v", decode)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

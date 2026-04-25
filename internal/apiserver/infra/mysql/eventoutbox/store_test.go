package eventoutbox

import (
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type fakeTopicResolver struct {
	topics     map[string]string
	deliveries map[string]eventcatalog.DeliveryClass
}

func (r fakeTopicResolver) GetTopicForEvent(eventType string) (string, bool) {
	topic, ok := r.topics[eventType]
	return topic, ok
}

func (r fakeTopicResolver) GetDeliveryClass(eventType string) (eventcatalog.DeliveryClass, bool) {
	delivery, ok := r.deliveries[eventType]
	return delivery, ok
}

func TestBuildRowsUsesInjectedTopicResolver(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	store := &Store{
		topicResolver: fakeTopicResolver{
			topics:     map[string]string{"sample.created": "sample.topic"},
			deliveries: map[string]eventcatalog.DeliveryClass{"sample.created": eventcatalog.DeliveryClassDurableOutbox},
		},
	}
	evt := event.New("sample.created", "Sample", "sample-1", map[string]string{"id": "sample-1"})

	rows, err := store.buildRowsAt([]event.DomainEvent{evt}, now)
	if err != nil {
		t.Fatalf("buildRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].TopicName != "sample.topic" {
		t.Fatalf("topic = %q, want sample.topic", rows[0].TopicName)
	}
	if rows[0].Status != outboxcore.StatusPending || rows[0].AttemptCount != 0 {
		t.Fatalf("initial state = %q/%d, want pending/0", rows[0].Status, rows[0].AttemptCount)
	}
	if !rows[0].NextAttemptAt.Equal(now) || !rows[0].CreatedAt.Equal(now) || !rows[0].UpdatedAt.Equal(now) {
		t.Fatalf("times = %#v, want %s", rows[0], now)
	}
	decoded, err := eventcodec.DecodeDomainEvent([]byte(rows[0].PayloadJSON))
	if err != nil {
		t.Fatalf("DecodeDomainEvent: %v", err)
	}
	if decoded.EventType() != evt.EventType() || decoded.AggregateID() != evt.AggregateID() {
		t.Fatalf("decoded event = %#v, want %q/%q", decoded, evt.EventType(), evt.AggregateID())
	}
}

func TestBuildRowsRejectsUnknownEvent(t *testing.T) {
	store := &Store{topicResolver: fakeTopicResolver{}}
	evt := event.New("sample.created", "Sample", "sample-1", map[string]string{})

	_, err := store.buildRows([]event.DomainEvent{evt})
	if err == nil {
		t.Fatalf("buildRows should reject unknown event")
	}
	if !strings.Contains(err.Error(), "sample.created") {
		t.Fatalf("error = %v, want event type", err)
	}
}

func TestBuildRowsRejectsBestEffortEvent(t *testing.T) {
	store := &Store{
		topicResolver: fakeTopicResolver{
			topics:     map[string]string{"sample.changed": "sample.topic"},
			deliveries: map[string]eventcatalog.DeliveryClass{"sample.changed": eventcatalog.DeliveryClassBestEffort},
		},
	}
	evt := event.New("sample.changed", "Sample", "sample-1", map[string]string{})

	_, err := store.buildRows([]event.DomainEvent{evt})
	if err == nil {
		t.Fatalf("buildRows should reject best-effort event")
	}
	if !strings.Contains(err.Error(), "best_effort") {
		t.Fatalf("error = %v, want delivery class", err)
	}
}

func TestOutboxStatusSnapshotNilStoreReturnsZeroBuckets(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	snapshot, err := (*Store)(nil).OutboxStatusSnapshot(t.Context(), now)
	if err != nil {
		t.Fatalf("OutboxStatusSnapshot: %v", err)
	}
	if snapshot.Store != "assessment-mysql-outbox" {
		t.Fatalf("store = %q, want assessment-mysql-outbox", snapshot.Store)
	}
	if len(snapshot.Buckets) != 3 {
		t.Fatalf("buckets = %#v, want three unfinished buckets", snapshot.Buckets)
	}
	for _, bucket := range snapshot.Buckets {
		if bucket.Count != 0 || bucket.OldestAgeSeconds != 0 {
			t.Fatalf("bucket = %#v, want zero bucket", bucket)
		}
	}
}

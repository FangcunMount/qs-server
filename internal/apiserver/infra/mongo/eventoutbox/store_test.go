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

func TestBuildDocumentsUsesInjectedTopicResolver(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 30, 0, 0, time.UTC)
	store := &Store{
		topicResolver: fakeTopicResolver{
			topics:     map[string]string{"sample.created": "sample.topic"},
			deliveries: map[string]eventcatalog.DeliveryClass{"sample.created": eventcatalog.DeliveryClassDurableOutbox},
		},
	}
	evt := event.New("sample.created", "Sample", "sample-1", map[string]string{"id": "sample-1"})

	docs, err := store.buildDocumentsAt([]event.DomainEvent{evt}, now)
	if err != nil {
		t.Fatalf("buildDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs len = %d, want 1", len(docs))
	}
	if docs[0].TopicName != "sample.topic" {
		t.Fatalf("topic = %q, want sample.topic", docs[0].TopicName)
	}
	if docs[0].Status != outboxcore.StatusPending || docs[0].AttemptCount != 0 {
		t.Fatalf("initial state = %q/%d, want pending/0", docs[0].Status, docs[0].AttemptCount)
	}
	if !docs[0].NextAttemptAt.Equal(now) || !docs[0].CreatedAt.Equal(now) || !docs[0].UpdatedAt.Equal(now) {
		t.Fatalf("times = %#v, want %s", docs[0], now)
	}
	decoded, err := eventcodec.DecodeDomainEvent([]byte(docs[0].PayloadJSON))
	if err != nil {
		t.Fatalf("DecodeDomainEvent: %v", err)
	}
	if decoded.EventType() != evt.EventType() || decoded.AggregateID() != evt.AggregateID() {
		t.Fatalf("decoded event = %#v, want %q/%q", decoded, evt.EventType(), evt.AggregateID())
	}
}

func TestBuildDocumentsRejectsUnknownEvent(t *testing.T) {
	store := &Store{topicResolver: fakeTopicResolver{}}
	evt := event.New("sample.created", "Sample", "sample-1", map[string]string{})

	_, err := store.buildDocuments([]event.DomainEvent{evt})
	if err == nil {
		t.Fatalf("buildDocuments should reject unknown event")
	}
	if !strings.Contains(err.Error(), "sample.created") {
		t.Fatalf("error = %v, want event type", err)
	}
}

func TestBuildDocumentsRejectsBestEffortEvent(t *testing.T) {
	store := &Store{
		topicResolver: fakeTopicResolver{
			topics:     map[string]string{"sample.changed": "sample.topic"},
			deliveries: map[string]eventcatalog.DeliveryClass{"sample.changed": eventcatalog.DeliveryClassBestEffort},
		},
	}
	evt := event.New("sample.changed", "Sample", "sample-1", map[string]string{})

	_, err := store.buildDocuments([]event.DomainEvent{evt})
	if err == nil {
		t.Fatalf("buildDocuments should reject best-effort event")
	}
	if !strings.Contains(err.Error(), "best_effort") {
		t.Fatalf("error = %v, want delivery class", err)
	}
}

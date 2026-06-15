package eventoutbox

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
	"go.mongodb.org/mongo-driver/bson"
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

func TestStageRequiresActiveSessionTransactionContext(t *testing.T) {
	store := &Store{}

	err := store.Stage(t.Context())
	if !errors.Is(err, ErrActiveSessionTransactionRequired) {
		t.Fatalf("Stage error = %v, want ErrActiveSessionTransactionRequired", err)
	}
}

func TestPendingClaimQueriesPrioritizeMainlineEvents(t *testing.T) {
	now := time.Date(2026, 6, 15, 19, 30, 0, 0, time.UTC)

	queries := pendingClaimQueries(now, defaultPriorityEventTypes())

	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	wantPriority := []string{
		eventcatalog.AnswerSheetSubmitted,
		eventcatalog.AssessmentInterpreted,
		eventcatalog.ReportGenerated,
	}
	assertEventTypeOperator(t, queries[0].filter, "$in", wantPriority)
	assertEventTypeOperator(t, queries[1].filter, "$nin", wantPriority)
	for _, query := range queries {
		if query.filter["status"] != outboxcore.StatusPending {
			t.Fatalf("filter status = %#v, want pending", query.filter["status"])
		}
		if _, ok := query.filter["next_attempt_at"].(bson.M)["$lte"]; !ok {
			t.Fatalf("filter next_attempt_at = %#v, want $lte", query.filter["next_attempt_at"])
		}
		if !reflect.DeepEqual(query.sort, bson.D{{Key: "created_at", Value: 1}}) {
			t.Fatalf("sort = %#v, want created_at asc", query.sort)
		}
	}
}

func TestPendingClaimQueriesFallsBackToFIFOWithoutPriority(t *testing.T) {
	now := time.Date(2026, 6, 15, 19, 30, 0, 0, time.UTC)

	queries := pendingClaimQueries(now, nil)

	if len(queries) != 1 {
		t.Fatalf("query count = %d, want 1", len(queries))
	}
	if _, ok := queries[0].filter["event_type"]; ok {
		t.Fatalf("filter event_type = %#v, want absent", queries[0].filter["event_type"])
	}
}

func assertEventTypeOperator(t *testing.T, filter bson.M, operator string, want []string) {
	t.Helper()
	eventTypeFilter, ok := filter["event_type"].(bson.M)
	if !ok {
		t.Fatalf("event_type filter = %#v, want bson.M", filter["event_type"])
	}
	got, ok := eventTypeFilter[operator].([]string)
	if !ok {
		t.Fatalf("event_type %s = %#v, want []string", operator, eventTypeFilter[operator])
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event_type %s = %#v, want %#v", operator, got, want)
	}
}

func TestOutboxStatusSnapshotNilStoreReturnsZeroBuckets(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	snapshot, err := (*Store)(nil).OutboxStatusSnapshot(t.Context(), now)
	if err != nil {
		t.Fatalf("OutboxStatusSnapshot: %v", err)
	}
	if snapshot.Store != "mongo-domain-events" {
		t.Fatalf("store = %q, want mongo-domain-events", snapshot.Store)
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

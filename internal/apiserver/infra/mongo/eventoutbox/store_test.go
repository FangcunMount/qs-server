package eventoutbox

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/pkg/event"
)

type fakeTopicResolver map[string]string

func (r fakeTopicResolver) GetTopicForEvent(eventType string) (string, bool) {
	topic, ok := r[eventType]
	return topic, ok
}

func TestBuildDocumentsUsesInjectedTopicResolver(t *testing.T) {
	store := &Store{
		topicResolver: fakeTopicResolver{"sample.created": "sample.topic"},
	}
	evt := event.New("sample.created", "Sample", "sample-1", map[string]string{"id": "sample-1"})

	docs, err := store.buildDocuments([]event.DomainEvent{evt})
	if err != nil {
		t.Fatalf("buildDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs len = %d, want 1", len(docs))
	}
	if docs[0].TopicName != "sample.topic" {
		t.Fatalf("topic = %q, want sample.topic", docs[0].TopicName)
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

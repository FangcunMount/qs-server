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

func TestBuildRowsUsesInjectedTopicResolver(t *testing.T) {
	store := &Store{
		topicResolver: fakeTopicResolver{"sample.created": "sample.topic"},
	}
	evt := event.New("sample.created", "Sample", "sample-1", map[string]string{"id": "sample-1"})

	rows, err := store.buildRows([]event.DomainEvent{evt})
	if err != nil {
		t.Fatalf("buildRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].TopicName != "sample.topic" {
		t.Fatalf("topic = %q, want sample.topic", rows[0].TopicName)
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

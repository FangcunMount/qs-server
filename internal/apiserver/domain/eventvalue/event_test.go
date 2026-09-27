package eventvalue

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	legacy "github.com/FangcunMount/component-base/pkg/event"
)

func TestEventValuePreservesHistoricalJSONAndDomainIdentity(t *testing.T) {
	at := time.Date(2026, 9, 27, 20, 30, 0, 123000000, time.FixedZone("UTC+8", 8*3600))
	base := BaseEvent{ID: "original-id", EventTypeValue: "questionnaire.changed", OccurredAtValue: at, AggregateTypeValue: "Questionnaire", AggregateIDValue: "问卷-A"}
	value := Event[map[string]string]{BaseEvent: base, Data: map[string]string{"title": "青岛测评"}}
	old := legacy.Event[map[string]string]{BaseEvent: legacy.BaseEvent{
		ID: base.ID, EventTypeValue: base.EventTypeValue, OccurredAtValue: base.OccurredAtValue,
		AggregateTypeValue: base.AggregateTypeValue, AggregateIDValue: base.AggregateIDValue,
	}, Data: value.Data}
	want, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("business event JSON changed: got %s, want %s", got, want)
	}
	if value.EventID() != old.EventID() || value.EventType() != old.EventType() || !value.OccurredAt().Equal(old.OccurredAt()) || value.AggregateType() != old.AggregateType() || value.AggregateID() != old.AggregateID() || value.Payload()["title"] != old.Payload()["title"] {
		t.Fatal("business event methods changed")
	}
}

func TestNewEventKeepsApplicationIdentityAndTimestamp(t *testing.T) {
	value := New("task.opened", "AssessmentTask", "task-1", struct{}{})
	if value.EventID() == "" || value.EventType() != "task.opened" || value.AggregateType() != "AssessmentTask" || value.AggregateID() != "task-1" || value.OccurredAt().IsZero() {
		t.Fatalf("invalid new business event: %#v", value)
	}
}

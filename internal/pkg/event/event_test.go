package event

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"testing"
	"time"
)

// Captured from 96fc4df42 with its original component-base dependency.
// Regenerate using scripts/testing/capture-retired-event-json.sh.
//
//go:embed testdata/retired-event.json
var retiredEventJSON []byte

func TestEventValuePreservesHistoricalJSONAndDomainIdentity(t *testing.T) {
	at := time.Date(2026, 9, 27, 20, 30, 0, 123000000, time.FixedZone("UTC+8", 8*3600))
	base := BaseEvent{ID: "original-id", EventTypeValue: "questionnaire.changed", OccurredAtValue: at, AggregateTypeValue: "Questionnaire", AggregateIDValue: "问卷-A"}
	value := Event[map[string]string]{BaseEvent: base, Data: map[string]string{"title": "青岛测评"}}
	want := retiredEventJSON
	got, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("business event JSON changed: got %s, want %s", got, want)
	}
	if value.EventID() != "original-id" || value.EventType() != "questionnaire.changed" || !value.OccurredAt().Equal(at) || value.AggregateType() != "Questionnaire" || value.AggregateID() != "问卷-A" || value.Payload()["title"] != "青岛测评" {
		t.Fatal("business event methods changed")
	}
}

func TestNewEventKeepsApplicationIdentityAndTimestamp(t *testing.T) {
	value := New("task.opened", "AssessmentTask", "task-1", struct{}{})
	if value.EventID() == "" || value.EventType() != "task.opened" || value.AggregateType() != "AssessmentTask" || value.AggregateID() != "task-1" || value.OccurredAt().IsZero() {
		t.Fatalf("invalid new business event: %#v", value)
	}
}

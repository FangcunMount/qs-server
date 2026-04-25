package eventcodec

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

type samplePayload struct {
	Value string `json:"value"`
}

func TestDomainEventJSONRoundTrip(t *testing.T) {
	evt := event.New("sample.created", "Sample", "sample-1", samplePayload{Value: "ok"})

	payload, err := EncodeDomainEvent(evt)
	if err != nil {
		t.Fatalf("EncodeDomainEvent: %v", err)
	}

	env, err := DecodeEnvelope(payload)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if env.ID != evt.EventID() || env.EventType != evt.EventType() {
		t.Fatalf("decoded envelope mismatch: %#v", env)
	}
	var data samplePayload
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.Value != "ok" {
		t.Fatalf("decoded data = %q, want ok", data.Value)
	}

	decoded, err := DecodeDomainEvent(payload)
	if err != nil {
		t.Fatalf("DecodeDomainEvent: %v", err)
	}
	if decoded.EventType() != evt.EventType() || decoded.AggregateID() != evt.AggregateID() {
		t.Fatalf("decoded domain event mismatch: %#v", decoded)
	}
}

func TestBuildMessageMetadata(t *testing.T) {
	evt := event.Event[samplePayload]{
		BaseEvent: event.BaseEvent{
			ID:                 "evt-1",
			EventTypeValue:     "sample.created",
			OccurredAtValue:    time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC),
			AggregateTypeValue: "Sample",
			AggregateIDValue:   "sample-1",
		},
		Data: samplePayload{Value: "ok"},
	}

	msg, err := BuildMessage(evt, "unit-test")
	if err != nil {
		t.Fatalf("BuildMessage: %v", err)
	}
	if msg.UUID != "evt-1" {
		t.Fatalf("message UUID = %q, want evt-1", msg.UUID)
	}
	if msg.Metadata["event_type"] != "sample.created" {
		t.Fatalf("event_type metadata = %q", msg.Metadata["event_type"])
	}
	if msg.Metadata["source"] != "unit-test" {
		t.Fatalf("source metadata = %q", msg.Metadata["source"])
	}
	if len(msg.Payload) == 0 {
		t.Fatalf("payload is empty")
	}
}

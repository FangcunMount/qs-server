package eventruntime

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/eventcodec"
	"github.com/FangcunMount/component-base/pkg/eventmessaging"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

// These are the existing QS producer and consumer codecs. An SDK publisher
// adapter must preserve this wire shape and the stored destination.
func TestDurableEventWireCompatibility(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resolver := eventcatalog.NewCatalog(cfg)
	for _, tc := range []struct {
		typeName, aggregate string
	}{
		{"answersheet.submitted", "AnswerSheet"},
		{"evaluation.requested", "Evaluation"},
	} {
		t.Run(tc.typeName, func(t *testing.T) {
			topic, ok := resolver.GetTopicForEvent(tc.typeName)
			if !ok || topic != "qs.evaluation.lifecycle" {
				t.Fatalf("topic = %q, found = %v", topic, ok)
			}
			evt := event.Event[map[string]any]{
				BaseEvent: event.BaseEvent{
					ID:                 "stable-event-id",
					EventTypeValue:     tc.typeName,
					OccurredAtValue:    time.Date(2026, 9, 23, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)),
					AggregateTypeValue: tc.aggregate,
					AggregateIDValue:   "aggregate-1",
				},
				Data: map[string]any{"org_id": 1, "answer_sheet_id": "answer-1"},
			}
			oldMessage, err := eventmessaging.BuildMessage(evt, SourceAPIServer)
			if err != nil {
				t.Fatal(err)
			}
			wire, err := messaging.EncodeMessagePayload(oldMessage)
			if err != nil {
				t.Fatal(err)
			}
			decoded, recognized, err := messaging.DecodeMessagePayload(wire)
			if err != nil || !recognized {
				t.Fatalf("transport decode: recognized=%v err=%v", recognized, err)
			}
			if decoded.UUID != evt.EventID() || decoded.Metadata["event_type"] != tc.typeName || decoded.Metadata["source"] != SourceAPIServer || decoded.Metadata["aggregate_id"] != evt.AggregateID() || decoded.Metadata["occurred_at"] != "2026-09-23T10:00:00.000+08:00" {
				t.Fatalf("identity or metadata changed: UUID=%q metadata=%v", decoded.UUID, decoded.Metadata)
			}
			if !bytes.Equal(decoded.Payload, oldMessage.Payload) {
				t.Fatal("transport changed the original domain envelope bytes")
			}
			workerEnvelope, err := eventcodec.DecodeEnvelope(decoded.Payload)
			if err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			if err := json.Unmarshal(workerEnvelope.Data, &data); err != nil {
				t.Fatal(err)
			}
			if workerEnvelope.ID != evt.EventID() || workerEnvelope.EventType != tc.typeName || workerEnvelope.AggregateID != evt.AggregateID() || data["answer_sheet_id"] != "answer-1" {
				t.Fatalf("worker envelope changed: %#v data=%v", workerEnvelope, data)
			}
		})
	}
}

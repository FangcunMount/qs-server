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
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/message"
)

// These are the existing QS producer and consumer codecs. An SDK publisher
// adapter must preserve this wire shape and the stored destination.
func TestDurableEventWireCompatibility(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resolver := eventcatalog.NewCatalog(cfg)
	cases := []struct {
		typeName, aggregate string
	}{
		{"answersheet.submitted", "AnswerSheet"},
		{"evaluation.requested", "Evaluation"},
		{"evaluation.retry.requested", "Evaluation"},
		{"evaluation.outcome.committed", "Evaluation"},
		{"evaluation.failed", "Evaluation"},
		{"interpretation.report.generated", "Report"},
		{"interpretation.report.failed", "Report"},
		{"interpretation.retry.requested", "ReportGeneration"},
	}
	durable := make(map[string]bool)
	for eventType, configured := range cfg.Events {
		if configured.Delivery == eventcatalog.DeliveryClassDurableOutbox {
			durable[eventType] = true
		}
	}
	if len(durable) != len(cases) {
		t.Fatalf("durable event inventory changed: catalog=%d protected=%d", len(durable), len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.typeName, func(t *testing.T) {
			if !durable[tc.typeName] {
				t.Fatalf("event %q is no longer configured as durable", tc.typeName)
			}
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
			if _, recognized, err := messaging.DecodeMessagePayload(oldMessage.Payload); err != nil || recognized {
				t.Fatalf("domain JSON alone unexpectedly carries transport identity: recognized=%v err=%v", recognized, err)
			}
			wire, err := messaging.EncodeMessagePayload(oldMessage)
			if err != nil {
				t.Fatal(err)
			}
			newWire, err := standardoutbox.EncodeWire(evt, SourceAPIServer)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(newWire, wire) {
				t.Fatal("standard Outbox changed the original NSQ wire bytes")
			}
			// The SDK publisher emits Payload verbatim; durable content is the
			// transport envelope, while SDK fields retain its stable identity.
			intent, err := message.New(message.Input{
				Producer: "qs-server", ID: evt.EventID(), Destination: topic,
				EventType: tc.typeName, SchemaVersion: "1", Scope: "org:1",
				ContentType: "application/json", OccurredAt: evt.OccurredAt().Format(time.RFC3339Nano), Payload: newWire,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(intent.Input().Payload, wire) {
				t.Fatal("SDK intent did not retain the original NSQ wire")
			}
			// The legacy Relay decodes the stored domain event and encodes it
			// again before publishing. The new intent must match that path too.
			stored, err := outboxcore.DecodePendingEvent(evt.EventID(), string(oldMessage.Payload))
			if err != nil {
				t.Fatal(err)
			}
			relayMessage, err := eventmessaging.BuildMessage(stored.Event, SourceAPIServer)
			if err != nil {
				t.Fatal(err)
			}
			relayWire, err := messaging.EncodeMessagePayload(relayMessage)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(newWire, relayWire) {
				t.Fatal("standard Outbox changed the legacy Relay wire bytes")
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

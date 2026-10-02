package eventruntime

import (
	"bytes"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// The six direct-publish flows must retain the same domain JSON and NSQ bytes
// when the Worker handler is moved from the old Message to SDK Delivery.
func TestBestEffortEventWireCompatibility(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ eventType, aggregate, topic string }{
		{"questionnaire.changed", "Questionnaire", "qs.survey.lifecycle"},
		{"assessment_model.changed", "AssessmentModel", "qs.survey.lifecycle"},
		{"task.opened", "AssessmentTask", "qs.plan.task"},
		{"task.completed", "AssessmentTask", "qs.plan.task"},
		{"task.expired", "AssessmentTask", "qs.plan.task"},
		{"task.canceled", "AssessmentTask", "qs.plan.task"},
	}
	bestEffortCount := 0
	for _, configured := range cfg.Events {
		if configured.Delivery == eventcatalog.DeliveryClassBestEffort {
			bestEffortCount++
		}
	}
	if bestEffortCount != len(cases) || len(cfg.Events) != 15 {
		t.Fatalf("event inventory changed: best_effort=%d protected=%d total=%d", bestEffortCount, len(cases), len(cfg.Events))
	}
	resolver := eventcatalog.NewCatalog(cfg)
	wires, _ := retiredWireContracts(t)
	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			if cfg.Events[tc.eventType].Delivery != eventcatalog.DeliveryClassBestEffort {
				t.Fatalf("%q is no longer best effort", tc.eventType)
			}
			if topic, ok := resolver.GetTopicForEvent(tc.eventType); !ok || topic != tc.topic {
				t.Fatalf("topic = %q, found = %v, want %q", topic, ok, tc.topic)
			}
			evt := event.Event[map[string]any]{
				BaseEvent: event.BaseEvent{
					ID: "best-effort-stable-id", EventTypeValue: tc.eventType,
					OccurredAtValue:    time.Date(2026, 9, 23, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)),
					AggregateTypeValue: tc.aggregate, AggregateIDValue: "aggregate-1",
				},
				Data: map[string]any{"org_id": 1, "revision": "v2"},
			}
			old, found := wires[tc.eventType]
			if !found {
				t.Fatal("missing historical wire")
			}
			newPayload, err := domainwire.EncodeEvent(evt)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(newPayload, []byte(old.Payload)) {
				t.Fatal("SDK changed the direct event JSON bytes")
			}
			newWire, err := legacy.Encode(legacy.Envelope{
				UUID: evt.EventID(), Metadata: domainwire.MetadataFromEvent(evt, SourceAPIServer),
				Payload: newPayload,
			}, legacy.Revision2)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(newWire, []byte(old.Wire)) {
				t.Fatal("SDK changed the direct NSQ wire bytes")
			}
			decoded, recognized, err := legacy.Decode(newWire)
			if err != nil || !recognized || decoded.UUID != evt.EventID() || decoded.Metadata["occurred_at"] != "2026-09-23T10:00:00.000+08:00" {
				t.Fatalf("SDK direct event identity changed: recognized=%v err=%v decoded=%#v", recognized, err, decoded)
			}
		})
	}
}

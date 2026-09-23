//go:build reliable_messaging_m4

package standardoutbox

import (
	"bytes"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

func TestPrepareIntentsCoversCurrentDurableCatalog(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resolver := eventcatalog.NewCatalog(cfg)
	cases := []string{
		"answersheet.submitted", "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed",
		"evaluation.failed", "interpretation.report.generated", "interpretation.report.failed", "interpretation.retry.requested",
	}
	var durableCount int
	for _, spec := range cfg.Events {
		if spec.Delivery == eventcatalog.DeliveryClassDurableOutbox {
			durableCount++
		}
	}
	if durableCount != len(cases) {
		t.Fatalf("durable catalog changed: configured=%d protected=%d", durableCount, len(cases))
	}
	for _, eventType := range cases {
		t.Run(eventType, func(t *testing.T) {
			evt := event.Event[map[string]any]{
				BaseEvent: event.BaseEvent{
					ID: "stable-" + eventType, EventTypeValue: eventType, AggregateTypeValue: "Proof", AggregateIDValue: "1",
					OccurredAtValue: time.Date(2026, 9, 23, 2, 0, 0, 0, time.UTC),
				},
				Data: map[string]any{"org_id": 501},
			}
			prepared, err := PrepareIntents([]event.DomainEvent{evt}, resolver, "api-server")
			if err != nil || len(prepared) != 1 {
				t.Fatalf("PrepareIntents() = %d, %v", len(prepared), err)
			}
			input := prepared[0].Message.Input()
			wire, err := EncodeWire(evt, "api-server")
			if err != nil {
				t.Fatal(err)
			}
			if input.ID != evt.EventID() || input.EventType != eventType || input.Scope != "org:501" || input.Destination != "qs.evaluation.lifecycle" ||
				input.OccurredAt != "2026-09-23T10:00:00+08:00" || !bytes.Equal(input.Payload, wire) {
				t.Fatalf("standard intent changed catalog, identity, scope, UTC+8 time or wire: %+v", input)
			}
		})
	}
}

func TestPrepareIntentsRejectsMissingScopeAndBestEffort(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resolver := eventcatalog.NewCatalog(cfg)
	for _, tc := range []struct {
		eventType string
		data      map[string]any
	}{
		{eventType: "answersheet.submitted", data: map[string]any{}},
		{eventType: "questionnaire.changed", data: map[string]any{"org_id": 501}},
	} {
		evt := event.Event[map[string]any]{BaseEvent: event.BaseEvent{
			ID: "invalid", EventTypeValue: tc.eventType, AggregateTypeValue: "Proof", AggregateIDValue: "1", OccurredAtValue: time.Now(),
		}, Data: tc.data}
		if prepared, err := PrepareIntents([]event.DomainEvent{evt}, resolver, "api-server"); err == nil || len(prepared) != 0 {
			t.Fatalf("%s prepared without durable scope: %d, %v", tc.eventType, len(prepared), err)
		}
	}
}

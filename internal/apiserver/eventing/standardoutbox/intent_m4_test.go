package standardoutbox

import (
	"bytes"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

func TestPrepareIntentsCoversCurrentDurableCatalog(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resolver := eventcatalog.NewCatalog(cfg)
	cases := []struct{ eventType, topic string }{
		{"answersheet.submitted", "qs.evaluation.lifecycle"},
		{"evaluation.requested", "qs.evaluation.lifecycle"},
		{"evaluation.retry.requested", "qs.evaluation.lifecycle"},
		{"evaluation.outcome.committed", "qs.evaluation.lifecycle"},
		{"evaluation.failed", "qs.evaluation.lifecycle"},
		{"interpretation.report.generated", "qs.evaluation.lifecycle"},
		{"interpretation.report.failed", "qs.evaluation.lifecycle"},
		{"interpretation.retry.requested", "qs.evaluation.lifecycle"},
		{"task.opened.reminder.requested", "qs.plan.task"},
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
	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			evt := event.Event[map[string]any]{
				BaseEvent: event.BaseEvent{
					ID: "stable-" + tc.eventType, EventTypeValue: tc.eventType, AggregateTypeValue: "Proof", AggregateIDValue: "1",
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
			if input.ID != evt.EventID() || input.EventType != tc.eventType || input.Scope != "org:501" || input.Destination != tc.topic ||
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

func TestPrepareIntentsKeepsOneDueTimeForTransactionBatch(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	occurredAt := time.Date(2026, 9, 23, 2, 0, 0, 0, time.UTC)
	events := []event.DomainEvent{
		event.Event[map[string]any]{BaseEvent: event.BaseEvent{
			ID: "batch-1", EventTypeValue: eventcatalog.AnswerSheetSubmitted,
			AggregateTypeValue: "AnswerSheet", AggregateIDValue: "101", OccurredAtValue: occurredAt,
		}, Data: map[string]any{"org_id": 501}},
		event.Event[map[string]any]{BaseEvent: event.BaseEvent{
			ID: "batch-2", EventTypeValue: eventcatalog.AnswerSheetSubmitted,
			AggregateTypeValue: "AnswerSheet", AggregateIDValue: "102", OccurredAtValue: occurredAt,
		}, Data: map[string]any{"org_id": 502}},
	}
	started := time.Now()
	prepared, err := PrepareIntents(events, eventcatalog.NewCatalog(cfg), "api-server")
	finished := time.Now()
	if err != nil || len(prepared) != 2 {
		t.Fatalf("PrepareIntents() = %d, %v", len(prepared), err)
	}
	if !prepared[0].DueAt.Equal(prepared[1].DueAt) || prepared[0].DueAt.Before(started) || prepared[0].DueAt.After(finished) {
		t.Fatalf("batch due times = %s, %s; call window [%s, %s]", prepared[0].DueAt, prepared[1].DueAt, started, finished)
	}
	if prepared[0].Message.Input().Scope != "org:501" || prepared[1].Message.Input().Scope != "org:502" {
		t.Fatalf("batch scopes = %q, %q", prepared[0].Message.Input().Scope, prepared[1].Message.Input().Scope)
	}
}

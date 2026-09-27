package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

func TestParseEventEnvelopePreservesHistoricalWorkerPayload(t *testing.T) {
	const payload = `{"id":"event-1","eventType":"evaluation.requested","occurredAt":"2026-09-23T10:00:00+08:00","aggregateType":"Evaluation","aggregateId":"assessment-1","data":{"answer_sheet_id":"answer-1"},"historical_context":{"batch_id":"old"}}`
	env, err := ParseEventEnvelope([]byte(payload))
	if err != nil {
		t.Fatalf("decode stored worker event: %v", err)
	}
	if env.ID != "event-1" || env.EventType != "evaluation.requested" || env.AggregateType != "Evaluation" || env.AggregateID != "assessment-1" {
		t.Fatalf("stored event identity changed: %#v", env)
	}
	if got := env.OccurredAt.Format(time.RFC3339); got != "2026-09-23T10:00:00+08:00" {
		t.Fatalf("stored event time = %s", got)
	}
	if got := string(env.Data); got != `{"answer_sheet_id":"answer-1"}` {
		t.Fatalf("stored event data = %s", got)
	}
	if _, err := ParseEventEnvelope([]byte(`{"id":`)); err == nil {
		t.Fatal("malformed stored event unexpectedly decoded")
	}
}

func TestRegistryResolvesConfiguredEventHandlers(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load events catalog: %v", err)
	}
	registry := NewRegistry()

	for eventType, eventCfg := range cfg.Events {
		if !registry.Has(eventCfg.Handler) {
			t.Fatalf("event_type %q references unresolved handler %q", eventType, eventCfg.Handler)
		}
	}
}

func TestNewRegistryFromFactories_IgnoresNilAndCopiesSourceMap(t *testing.T) {
	factories := map[string]HandlerFactory{
		"valid": func(*Dependencies) HandlerFunc {
			return func(context.Context, string, []byte) error { return nil }
		},
		"nil_factory": nil,
	}

	registry := newRegistryFromFactories(factories)
	// mutate source map after creation, registry should remain unaffected.
	delete(factories, "valid")

	if registry.Has("nil_factory") {
		t.Fatal("nil factory should not be registered")
	}
	if !registry.Has("valid") {
		t.Fatal("valid factory should remain registered after source map mutation")
	}
}

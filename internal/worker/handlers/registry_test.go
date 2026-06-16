package handlers

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

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

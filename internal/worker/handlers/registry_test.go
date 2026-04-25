package handlers

import (
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

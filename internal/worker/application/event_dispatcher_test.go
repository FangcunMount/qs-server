package application

import (
	"io"
	"log/slog"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

func TestEventDispatcherInitializesCurrentRuntimeTopics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dispatcher := NewEventDispatcher(logger, &HandlerDependencies{
		Logger: logger,
	})

	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := dispatcher.Initialize(eventcatalog.NewCatalog(cfg)); err != nil {
		t.Fatalf("initialize dispatcher: %v", err)
	}

	subs := dispatcher.GetTopicSubscriptions()
	if len(subs) != 4 {
		t.Fatalf("expected 4 topic subscriptions, got %d", len(subs))
	}

	for _, eventType := range cfg.ListEventTypes() {
		if !dispatcher.HasHandler(eventType) {
			t.Fatalf("expected handler for event type %q", eventType)
		}
	}
}

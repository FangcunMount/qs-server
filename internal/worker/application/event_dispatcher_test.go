package application

import (
	"io"
	"log/slog"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
)

func TestEventDispatcherInitializesCurrentRuntimeTopics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dispatcher := NewEventDispatcher(logger, &HandlerDependencies{
		Logger: logger,
	})

	if err := dispatcher.Initialize("../../../configs/events.yaml"); err != nil {
		t.Fatalf("initialize dispatcher: %v", err)
	}

	subs := dispatcher.GetTopicSubscriptions()
	if len(subs) != 3 {
		t.Fatalf("expected 3 topic subscriptions, got %d", len(subs))
	}

	cfg, err := eventconfig.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	for _, eventType := range cfg.ListEventTypes() {
		if !dispatcher.HasHandler(eventType) {
			t.Fatalf("expected handler for event type %q", eventType)
		}
	}
}

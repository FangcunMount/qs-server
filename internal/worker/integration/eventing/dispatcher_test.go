package eventing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
)

type fakeHandlerRegistry struct {
	names    []string
	handlers map[string]handlers.HandlerFunc
	deps     *handlers.Dependencies
	created  []string
}

func (r *fakeHandlerRegistry) Names() []string {
	return append([]string(nil), r.names...)
}

func (r *fakeHandlerRegistry) Has(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

func (r *fakeHandlerRegistry) Create(name string, deps *handlers.Dependencies) (handlers.HandlerFunc, bool) {
	r.deps = deps
	handler, ok := r.handlers[name]
	if ok {
		r.created = append(r.created, name)
	}
	return handler, ok
}

func TestDispatcherInitializesCurrentRuntimeTopics(t *testing.T) {
	logger := testLogger()
	dispatcher := NewDispatcher(logger, &HandlerDependencies{
		Logger: logger,
	}, handlers.NewRegistry())

	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
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

func TestDispatcherRejectsNilCatalog(t *testing.T) {
	dispatcher := NewDispatcher(testLogger(), &HandlerDependencies{}, &fakeHandlerRegistry{})

	if err := dispatcher.Initialize(nil); err == nil || !strings.Contains(err.Error(), "event catalog is not loaded") {
		t.Fatalf("Initialize(nil) error = %v, want event catalog error", err)
	}
}

func TestDispatcherRejectsMissingHandler(t *testing.T) {
	dispatcher := NewDispatcher(testLogger(), &HandlerDependencies{}, &fakeHandlerRegistry{})

	err := dispatcher.Initialize(sampleCatalog("missing_handler"))
	if err == nil || !strings.Contains(err.Error(), `handler "missing_handler" not registered`) {
		t.Fatalf("Initialize error = %v, want missing handler error", err)
	}
}

func TestDispatcherUsesInjectedRegistry(t *testing.T) {
	var dispatched bool
	registry := &fakeHandlerRegistry{
		names: []string{"sample_handler"},
		handlers: map[string]handlers.HandlerFunc{
			"sample_handler": func(ctx context.Context, eventType string, payload []byte) error {
				dispatched = true
				if eventType != "sample.created" {
					t.Fatalf("eventType = %q, want sample.created", eventType)
				}
				if string(payload) != "payload" {
					t.Fatalf("payload = %q, want payload", payload)
				}
				return nil
			},
		},
	}
	dispatcher := NewDispatcher(testLogger(), &HandlerDependencies{Logger: testLogger()}, registry)

	if err := dispatcher.Initialize(sampleCatalog("sample_handler")); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if registry.deps == nil || registry.deps.Logger == nil {
		t.Fatalf("registry did not receive handler dependencies")
	}
	if got := registry.created; len(got) != 1 || got[0] != "sample_handler" {
		t.Fatalf("created handlers = %#v, want only sample_handler", got)
	}
	if !dispatcher.HasHandler("sample.created") {
		t.Fatalf("expected sample.created handler")
	}
	if got := dispatcher.GetTopicSubscriptions(); len(got) != 1 || got[0].TopicName != "sample.topic" {
		t.Fatalf("subscriptions = %#v, want sample.topic", got)
	}
	if err := dispatcher.DispatchEvent(context.Background(), "sample.created", []byte("payload")); err != nil {
		t.Fatalf("DispatchEvent: %v", err)
	}
	if !dispatched {
		t.Fatalf("handler was not dispatched")
	}
}

func TestDispatcherReturnsHandlerError(t *testing.T) {
	wantErr := errors.New("handler failed")
	registry := &fakeHandlerRegistry{
		handlers: map[string]handlers.HandlerFunc{
			"sample_handler": func(context.Context, string, []byte) error {
				return wantErr
			},
		},
	}
	dispatcher := NewDispatcher(testLogger(), &HandlerDependencies{}, registry)

	if err := dispatcher.Initialize(sampleCatalog("sample_handler")); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := dispatcher.DispatchEvent(context.Background(), "sample.created", []byte("payload")); !errors.Is(err, wantErr) {
		t.Fatalf("DispatchEvent error = %v, want %v", err, wantErr)
	}
}

func TestDispatcherCreatesOnlyCatalogReferencedHandlers(t *testing.T) {
	registry := &fakeHandlerRegistry{
		names: []string{"sample_handler", "unused_handler"},
		handlers: map[string]handlers.HandlerFunc{
			"sample_handler": func(context.Context, string, []byte) error {
				return nil
			},
			"unused_handler": func(context.Context, string, []byte) error {
				return nil
			},
		},
	}
	dispatcher := NewDispatcher(testLogger(), &HandlerDependencies{}, registry)

	if err := dispatcher.Initialize(sampleCatalog("sample_handler")); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if got := registry.created; len(got) != 1 || got[0] != "sample_handler" {
		t.Fatalf("created handlers = %#v, want only sample_handler", got)
	}
}

func sampleCatalog(handlerName string) *eventcatalog.Catalog {
	return eventcatalog.NewCatalog(&eventcatalog.Config{
		Topics: map[string]eventcatalog.TopicConfig{
			"sample": {Name: "sample.topic"},
		},
		Events: map[string]eventcatalog.EventConfig{
			"sample.created": {
				Topic:   "sample",
				Handler: handlerName,
			},
		},
	})
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

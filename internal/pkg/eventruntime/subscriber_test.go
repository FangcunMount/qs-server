package eventruntime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

func TestSubscriberGetTopicsToSubscribe(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	sub := NewSubscriber(SubscriberOptions{
		Catalog: eventcatalog.NewCatalog(cfg),
		HandlerFactory: func(_ string) (HandlerFunc, error) {
			return func(context.Context, string, []byte) error { return nil }, nil
		},
	})

	if err := sub.RegisterHandlers(); err != nil {
		t.Fatalf("register handlers: %v", err)
	}

	subs := sub.GetTopicsToSubscribe()
	if len(subs) != 3 {
		t.Fatalf("expected 3 topic subscriptions, got %d", len(subs))
	}
	for _, sub := range subs {
		if sub.TopicName == "" || sub.TopicKey == "" {
			t.Fatalf("invalid topic subscription: %#v", sub)
		}
		if len(sub.EventTypes) == 0 {
			t.Fatalf("topic subscription has no events: %#v", sub)
		}
	}
}

func TestSubscriberRegisterHandlersFailsOnMissingHandler(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	const missingHandler = "task_completed_handler"
	sub := NewSubscriber(SubscriberOptions{
		Catalog: eventcatalog.NewCatalog(cfg),
		HandlerFactory: func(handlerName string) (HandlerFunc, error) {
			if handlerName == missingHandler {
				return nil, errors.New("not found")
			}
			return func(context.Context, string, []byte) error { return nil }, nil
		},
	})

	err = sub.RegisterHandlers()
	if err == nil {
		t.Fatal("expected missing handler error")
	}
	if !strings.Contains(err.Error(), missingHandler) {
		t.Fatalf("expected error to mention %q, got %v", missingHandler, err)
	}
}

func TestSubscriberDispatchUnknownEventSkipsWithoutError(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	sub := NewSubscriber(SubscriberOptions{
		Catalog: eventcatalog.NewCatalog(cfg),
		HandlerFactory: func(_ string) (HandlerFunc, error) {
			return func(context.Context, string, []byte) error { return nil }, nil
		},
	})
	if err := sub.RegisterHandlers(); err != nil {
		t.Fatalf("register handlers: %v", err)
	}
	result, err := sub.Dispatch(t.Context(), "unknown.event", []byte(`{}`))
	if err != nil {
		t.Fatalf("unknown event dispatch = %v, want nil so messaging can ACK", err)
	}
	if result.Outcome != DispatchUnknown {
		t.Fatalf("outcome = %q, want unknown", result.Outcome)
	}
}

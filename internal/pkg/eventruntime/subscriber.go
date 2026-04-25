package eventruntime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

// HandlerFunc processes one event payload.
type HandlerFunc func(ctx context.Context, eventType string, payload []byte) error

// HandlerFactory creates handlers by configured handler name.
type HandlerFactory func(handlerName string) (HandlerFunc, error)

// Subscriber registers configured handlers and dispatches events by event type.
type Subscriber struct {
	catalog        catalogReader
	handlerFactory HandlerFactory
	logger         *slog.Logger
	handlers       map[string]HandlerFunc
}

type catalogReader interface {
	Config() *eventcatalog.Config
	TopicSubscriptions() []eventcatalog.TopicSubscription
}

// SubscriberOptions defines explicit runtime dependencies for subscription.
type SubscriberOptions struct {
	Catalog        *eventcatalog.Catalog
	HandlerFactory HandlerFactory
	Logger         *slog.Logger
}

// NewSubscriber creates a subscriber from an explicit catalog.
func NewSubscriber(opts SubscriberOptions) *Subscriber {
	var catalog catalogReader
	if opts.Catalog != nil {
		catalog = opts.Catalog
	}
	if catalog == nil {
		catalog = eventcatalog.NewCatalog(nil)
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Subscriber{
		catalog:        catalog,
		handlerFactory: opts.HandlerFactory,
		logger:         opts.Logger,
		handlers:       make(map[string]HandlerFunc),
	}
}

// RegisterHandlers creates all handlers referenced by the catalog.
func (s *Subscriber) RegisterHandlers() error {
	cfg := s.catalog.Config()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	for eventType, eventCfg := range cfg.Events {
		handler, err := s.handlerFactory(eventCfg.Handler)
		if err != nil {
			return fmt.Errorf("event %q references unavailable handler %q: %w", eventType, eventCfg.Handler, err)
		}
		s.handlers[eventType] = handler
		s.logger.Info("handler registered",
			slog.String("event_type", eventType),
			slog.String("handler", eventCfg.Handler),
		)
	}
	return nil
}

// GetTopicsToSubscribe returns configured topic subscriptions.
func (s *Subscriber) GetTopicsToSubscribe() []eventcatalog.TopicSubscription {
	return s.catalog.TopicSubscriptions()
}

// Dispatch dispatches an event to its configured handler.
func (s *Subscriber) Dispatch(ctx context.Context, eventType string, payload []byte) error {
	handler, ok := s.handlers[eventType]
	if !ok {
		s.logger.Warn("no handler for event type",
			slog.String("event_type", eventType),
		)
		return nil
	}
	return handler(ctx, eventType, payload)
}

// HasHandler reports whether an event type has a registered handler.
func (s *Subscriber) HasHandler(eventType string) bool {
	_, ok := s.handlers[eventType]
	return ok
}

// HandlerCount returns the number of registered handlers.
func (s *Subscriber) HandlerCount() int {
	return len(s.handlers)
}

// Package eventing adapts the worker handler registry to the shared event runtime.
package eventing

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/worker/port"
)

// HandlerDependencies are dependencies needed to instantiate worker event handlers.
type HandlerDependencies struct {
	Logger            *slog.Logger
	AnswerSheetClient *grpcclient.AnswerSheetClient
	EvaluationClient  *grpcclient.EvaluationClient
	InternalClient    handlers.InternalClient
	LockManager       *redislock.Manager
	LockKeyBuilder    *rediskey.Builder
	Notifier          port.TaskNotifier
}

// HandlerRegistry hides the worker handlers package's init-based registry behind
// a narrow seam so dispatcher tests do not need to mutate global handler state.
type HandlerRegistry interface {
	ListRegistered() []string
	CreateAll(*handlers.Dependencies) map[string]handlers.HandlerFunc
}

type defaultHandlerRegistry struct{}

func (defaultHandlerRegistry) ListRegistered() []string {
	return handlers.ListRegistered()
}

func (defaultHandlerRegistry) CreateAll(deps *handlers.Dependencies) map[string]handlers.HandlerFunc {
	return handlers.CreateAll(deps)
}

// Dispatcher subscribes configured event types and dispatches messages to worker handlers.
type Dispatcher struct {
	logger     *slog.Logger
	subscriber *eventruntime.Subscriber
	deps       *HandlerDependencies
	registry   HandlerRegistry
}

// NewDispatcher creates a dispatcher backed by the worker handlers package registry.
func NewDispatcher(logger *slog.Logger, deps *HandlerDependencies) *Dispatcher {
	return NewDispatcherWithRegistry(logger, deps, defaultHandlerRegistry{})
}

// NewDispatcherWithRegistry creates a dispatcher with an explicit handler registry.
func NewDispatcherWithRegistry(logger *slog.Logger, deps *HandlerDependencies, registry HandlerRegistry) *Dispatcher {
	if registry == nil {
		registry = defaultHandlerRegistry{}
	}
	return &Dispatcher{
		logger:   logger,
		deps:     deps,
		registry: registry,
	}
}

// Initialize initializes event subscriptions and handler bindings.
func (d *Dispatcher) Initialize(catalog *eventcatalog.Catalog) error {
	if catalog == nil || catalog.Config() == nil {
		return fmt.Errorf("event catalog is not loaded")
	}
	d.logger.Info("initializing event dispatcher")

	registeredHandlers := d.registry.ListRegistered()
	d.logger.Info("handlers registered via init()",
		slog.Int("count", len(registeredHandlers)),
		slog.Any("handlers", registeredHandlers),
	)

	factory := d.createHandlerFactory(d.buildHandlerDependencies())

	d.subscriber = eventruntime.NewSubscriber(eventruntime.SubscriberOptions{
		Catalog:        catalog,
		HandlerFactory: factory,
		Logger:         d.logger,
	})

	if err := d.subscriber.RegisterHandlers(); err != nil {
		return fmt.Errorf("failed to register handlers: %w", err)
	}

	d.logger.Info("event dispatcher initialized",
		slog.Int("handler_count", d.subscriber.HandlerCount()),
	)

	return nil
}

func (d *Dispatcher) buildHandlerDependencies() *handlers.Dependencies {
	return &handlers.Dependencies{
		Logger:            d.deps.Logger,
		AnswerSheetClient: d.deps.AnswerSheetClient,
		EvaluationClient:  d.deps.EvaluationClient,
		InternalClient:    d.deps.InternalClient,
		LockManager:       d.deps.LockManager,
		LockKeyBuilder:    d.deps.LockKeyBuilder,
		Notifier:          d.deps.Notifier,
	}
}

func (d *Dispatcher) createHandlerFactory(deps *handlers.Dependencies) eventruntime.HandlerFactory {
	allHandlers := d.registry.CreateAll(deps)
	return func(handlerName string) (eventruntime.HandlerFunc, error) {
		handler, ok := allHandlers[handlerName]
		if !ok {
			return nil, fmt.Errorf("handler %q not registered via init()", handlerName)
		}
		return eventruntime.HandlerFunc(handler), nil
	}
}

// GetTopicSubscriptions returns topics the worker should subscribe to.
func (d *Dispatcher) GetTopicSubscriptions() []eventcatalog.TopicSubscription {
	if d.subscriber == nil {
		return nil
	}
	return d.subscriber.GetTopicsToSubscribe()
}

// DispatchEvent dispatches an event to its configured handler.
func (d *Dispatcher) DispatchEvent(ctx context.Context, eventType string, payload []byte) error {
	if d.subscriber == nil {
		return fmt.Errorf("event dispatcher not initialized")
	}
	return d.subscriber.Dispatch(ctx, eventType, payload)
}

// HasHandler reports whether an event type has a registered handler.
func (d *Dispatcher) HasHandler(eventType string) bool {
	if d.subscriber == nil {
		return false
	}
	return d.subscriber.HasHandler(eventType)
}

// PrintSubscriptionInfo logs topic subscriptions for startup diagnostics.
func (d *Dispatcher) PrintSubscriptionInfo() {
	subs := d.GetTopicSubscriptions()

	d.logger.Info("=== Topic Subscriptions ===")
	for _, sub := range subs {
		d.logger.Info("topic subscription",
			slog.String("topic", sub.TopicName),
			slog.Int("event_count", len(sub.EventTypes)),
		)
		for _, eventType := range sub.EventTypes {
			hasHandler := "✗"
			if d.HasHandler(eventType) {
				hasHandler = "✓"
			}
			d.logger.Info("  event type",
				slog.String("event_type", eventType),
				slog.String("has_handler", hasHandler),
			)
		}
	}
}

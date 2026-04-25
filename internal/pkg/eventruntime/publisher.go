package eventruntime

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// RoutingPublisher routes domain events to topics described by an event catalog.
type RoutingPublisher struct {
	topicResolver eventcatalog.TopicResolver
	mqPublisher   messaging.Publisher
	observer      eventobservability.Observer
	source        string
	mode          PublishMode
}

// PublishMode controls the publisher side effect mode.
type PublishMode string

const (
	PublishModeMQ      PublishMode = "mq"
	PublishModeLogging PublishMode = "logging"
	PublishModeNop     PublishMode = "nop"
)

// PublishModeFromEnv maps process mode to a safe default publish mode.
func PublishModeFromEnv(env string) PublishMode {
	switch env {
	case "prod", "production":
		return PublishModeMQ
	case "dev", "development":
		return PublishModeLogging
	case "test", "testing":
		return PublishModeNop
	default:
		return PublishModeLogging
	}
}

// RoutingPublisherOptions defines explicit runtime dependencies for publishing.
type RoutingPublisherOptions struct {
	Catalog       *eventcatalog.Catalog
	TopicResolver eventcatalog.TopicResolver
	MQPublisher   messaging.Publisher
	Observer      eventobservability.Observer
	Source        string
	Mode          PublishMode
}

// NewRoutingPublisher creates a routing publisher.
func NewRoutingPublisher(opts RoutingPublisherOptions) *RoutingPublisher {
	resolver := opts.TopicResolver
	if resolver == nil && opts.Catalog != nil {
		resolver = opts.Catalog
	}
	if resolver == nil {
		resolver = eventcatalog.NewCatalog(nil)
	}
	if opts.Source == "" {
		opts.Source = event.SourceAPIServer
	}
	if opts.Mode == "" {
		opts.Mode = PublishModeLogging
	}
	observer := opts.Observer
	if observer == nil {
		observer = eventobservability.DefaultObserver()
	}
	return &RoutingPublisher{
		topicResolver: resolver,
		mqPublisher:   opts.MQPublisher,
		observer:      observer,
		source:        opts.Source,
		mode:          opts.Mode,
	}
}

// Publish publishes one event to its configured topic.
func (p *RoutingPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	eventType := evt.EventType()
	topicName, ok := p.topicResolver.GetTopicForEvent(eventType)
	if !ok {
		p.observe(ctx, "", eventType, eventobservability.PublishOutcomeUnknownEvent)
		logger.L(ctx).Errorw("event type not found in config, cannot route to topic",
			"event_type", eventType,
			"event_id", evt.EventID(),
			"aggregate_type", evt.AggregateType(),
			"aggregate_id", evt.AggregateID(),
		)
		return fmt.Errorf("event type %q not found in config", eventType)
	}

	logger.L(ctx).Debugw("routing event to topic",
		"event_type", eventType,
		"topic_name", topicName,
		"mode", p.mode,
	)

	switch p.mode {
	case PublishModeMQ:
		return p.publishToMQ(ctx, topicName, evt)
	case PublishModeLogging:
		p.publishToLog(ctx, topicName, evt)
		p.observe(ctx, topicName, evt.EventType(), eventobservability.PublishOutcomeLogged)
		return nil
	case PublishModeNop:
		p.observe(ctx, topicName, evt.EventType(), eventobservability.PublishOutcomeNop)
		return nil
	default:
		p.publishToLog(ctx, topicName, evt)
		p.observe(ctx, topicName, evt.EventType(), eventobservability.PublishOutcomeLogged)
		return nil
	}
}

// PublishAll publishes events sequentially and stops at first error.
func (p *RoutingPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func (p *RoutingPublisher) publishToMQ(ctx context.Context, topicName string, evt event.DomainEvent) error {
	if p.mqPublisher == nil {
		logger.L(ctx).Warnw("MQ publisher is nil, falling back to logging",
			"event_type", evt.EventType(),
			"topic", topicName,
		)
		p.publishToLog(ctx, topicName, evt)
		p.observe(ctx, topicName, evt.EventType(), eventobservability.PublishOutcomeFallbackLogged)
		return nil
	}

	msg, err := eventcodec.BuildMessage(evt, p.source)
	if err != nil {
		p.observe(ctx, topicName, evt.EventType(), eventobservability.PublishOutcomeEncodeFailed)
		return err
	}

	if err := p.mqPublisher.PublishMessage(ctx, topicName, msg); err != nil {
		p.observe(ctx, topicName, evt.EventType(), eventobservability.PublishOutcomeMQFailed)
		logger.L(ctx).Errorw("failed to publish event to topic",
			"event_type", evt.EventType(),
			"topic", topicName,
			"error", err.Error(),
		)
		return fmt.Errorf("failed to publish to topic %s: %w", topicName, err)
	}

	logger.L(ctx).Infow("event published to topic",
		"action", "publish_event",
		"event_type", evt.EventType(),
		"event_id", evt.EventID(),
		"topic", topicName,
		"source", p.source,
		"result", "success",
	)
	p.observe(ctx, topicName, evt.EventType(), eventobservability.PublishOutcomeMQPublished)
	return nil
}

func (p *RoutingPublisher) publishToLog(ctx context.Context, topicName string, evt event.DomainEvent) {
	logger.L(ctx).Infow("[DomainEvent]",
		"action", "log_event",
		"event_type", evt.EventType(),
		"event_id", evt.EventID(),
		"aggregate_type", evt.AggregateType(),
		"aggregate_id", evt.AggregateID(),
		"topic", topicName,
		"source", p.source,
	)
}

func (p *RoutingPublisher) observe(ctx context.Context, topicName, eventType string, outcome eventobservability.PublishOutcome) {
	if p == nil || p.observer == nil {
		return
	}
	p.observer.ObservePublish(ctx, eventobservability.PublishEvent{
		Source:    p.source,
		Mode:      string(p.mode),
		Topic:     topicName,
		EventType: eventType,
		Outcome:   outcome,
	})
}

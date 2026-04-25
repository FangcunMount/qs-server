package eventruntime

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// RoutingPublisher routes domain events to topics described by an event catalog.
type RoutingPublisher struct {
	topicResolver eventcatalog.TopicResolver
	mqPublisher   messaging.Publisher
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
	return &RoutingPublisher{
		topicResolver: resolver,
		mqPublisher:   opts.MQPublisher,
		source:        opts.Source,
		mode:          opts.Mode,
	}
}

// Publish publishes one event to its configured topic.
func (p *RoutingPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	eventType := evt.EventType()
	topicName, ok := p.topicResolver.GetTopicForEvent(eventType)
	if !ok {
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
		return p.publishToLog(ctx, topicName, evt)
	case PublishModeNop:
		return nil
	default:
		return p.publishToLog(ctx, topicName, evt)
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
		return p.publishToLog(ctx, topicName, evt)
	}

	msg, err := eventcodec.BuildMessage(evt, p.source)
	if err != nil {
		return err
	}

	if err := p.mqPublisher.PublishMessage(ctx, topicName, msg); err != nil {
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
	return nil
}

func (p *RoutingPublisher) publishToLog(ctx context.Context, topicName string, evt event.DomainEvent) error {
	logger.L(ctx).Infow("[DomainEvent]",
		"action", "log_event",
		"event_type", evt.EventType(),
		"event_id", evt.EventID(),
		"aggregate_type", evt.AggregateType(),
		"aggregate_id", evt.AggregateID(),
		"topic", topicName,
		"source", p.source,
	)
	return nil
}

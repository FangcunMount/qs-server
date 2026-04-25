package messaging

import (
	"context"
	"log/slog"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	"github.com/FangcunMount/component-base/pkg/messaging/rabbitmq"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/nsqio/go-nsq"
)

type eventDispatcher interface {
	DispatchEvent(ctx context.Context, eventType string, payload []byte) error
}

type MessageEventExtractor struct{}

func (MessageEventExtractor) Extract(msg *basemessaging.Message) (string, error) {
	if msg.Metadata == nil {
		msg.Metadata = map[string]string{}
	}
	if eventType, ok := msg.Metadata["event_type"]; ok {
		return eventType, nil
	}
	env, err := handlers.ParseEventEnvelope(msg.Payload)
	if err != nil {
		return "", err
	}
	msg.Metadata["event_type"] = env.EventType
	return env.EventType, nil
}

type MessageSettlementPolicy struct {
	logger *slog.Logger
	topic  string
}

func (p MessageSettlementPolicy) AckInvalid(msg *basemessaging.Message, parseErr error) {
	p.logger.Warn("message missing event_type and payload parse failed",
		slog.String("topic", p.topic),
		slog.String("msg_id", msg.UUID),
		slog.String("error", parseErr.Error()),
	)
	if ackErr := msg.Ack(); ackErr != nil {
		p.logger.Warn("failed to ack invalid message",
			slog.String("topic", p.topic),
			slog.String("msg_id", msg.UUID),
			slog.String("error", ackErr.Error()),
		)
	}
}

func (p MessageSettlementPolicy) NackFailed(msg *basemessaging.Message, eventType string, dispatchErr error) {
	p.logger.Error("failed to dispatch event",
		slog.String("topic", p.topic),
		slog.String("event_type", eventType),
		slog.String("msg_id", msg.UUID),
		slog.String("error", dispatchErr.Error()),
	)
	if nackErr := msg.Nack(); nackErr != nil {
		p.logger.Warn("failed to nack message",
			slog.String("topic", p.topic),
			slog.String("msg_id", msg.UUID),
			slog.String("error", nackErr.Error()),
		)
	}
}

func (p MessageSettlementPolicy) AckSuccess(msg *basemessaging.Message) error {
	if ackErr := msg.Ack(); ackErr != nil {
		p.logger.Warn("failed to ack message",
			slog.String("topic", p.topic),
			slog.String("msg_id", msg.UUID),
			slog.String("error", ackErr.Error()),
		)
		return ackErr
	}
	return nil
}

func CreateSubscriber(cfg *config.MessagingConfig, logger *slog.Logger, maxInFlight int) (basemessaging.Subscriber, error) {
	switch cfg.Provider {
	case "nsq":
		nsqCfg := nsq.NewConfig()
		if maxInFlight > 0 {
			nsqCfg.MaxInFlight = maxInFlight
		}
		return cbnsq.NewSubscriber([]string{cfg.NSQLookupdAddr}, nsqCfg)
	case "rabbitmq":
		return rabbitmq.NewSubscriber(cfg.RabbitMQURL)
	default:
		logger.Warn("unknown messaging provider, using NSQ as default",
			slog.String("provider", cfg.Provider),
		)
		return cbnsq.NewSubscriber([]string{cfg.NSQLookupdAddr}, nil)
	}
}

func EnsureTopics(cfg *config.MessagingConfig, logger *slog.Logger, c *container.Container) error {
	if c == nil {
		return nil
	}
	subscriptions := c.GetTopicSubscriptions()
	topics := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		topics = append(topics, sub.TopicName)
	}

	if len(topics) == 0 {
		logger.Debug("No topics to create")
		return nil
	}

	creator := cbnsq.NewTopicCreator(cfg.NSQAddr, logger)
	return creator.EnsureTopics(topics)
}

func SubscribeHandlers(serviceName string, logger *slog.Logger, c *container.Container, subscriber basemessaging.Subscriber) error {
	if c == nil || subscriber == nil {
		return nil
	}

	subscriptions := c.GetTopicSubscriptions()
	for _, sub := range subscriptions {
		topicName := sub.TopicName
		msgHandler := createDispatchHandler(logger, c, topicName)
		if err := subscriber.Subscribe(topicName, serviceName, msgHandler); err != nil {
			logger.Error("failed to subscribe",
				slog.String("topic", topicName),
				slog.String("error", err.Error()),
			)
			return err
		}
		logger.Info("subscribed to topic",
			slog.String("topic", topicName),
			slog.Int("event_count", len(sub.EventTypes)),
			slog.String("channel", serviceName),
		)
	}
	return nil
}

func createDispatchHandler(logger *slog.Logger, dispatcher eventDispatcher, topicName string) basemessaging.Handler {
	extractor := MessageEventExtractor{}
	settlement := MessageSettlementPolicy{logger: logger, topic: topicName}
	return func(ctx context.Context, msg *basemessaging.Message) error {
		eventType, err := extractor.Extract(msg)
		if err != nil {
			settlement.AckInvalid(msg, err)
			return nil
		}

		logger.Debug("received message",
			slog.String("topic", topicName),
			slog.String("event_type", eventType),
			slog.String("msg_id", msg.UUID),
		)

		if err := dispatcher.DispatchEvent(ctx, eventType, msg.Payload); err != nil {
			settlement.NackFailed(msg, eventType, err)
			return err
		}

		return settlement.AckSuccess(msg)
	}
}

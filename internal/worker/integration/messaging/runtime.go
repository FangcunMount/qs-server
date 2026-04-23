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

func createDispatchHandler(logger *slog.Logger, c *container.Container, topicName string) basemessaging.Handler {
	return func(ctx context.Context, msg *basemessaging.Message) error {
		eventType, ok := msg.Metadata["event_type"]
		if !ok {
			env, err := handlers.ParseEventEnvelope(msg.Payload)
			if err != nil {
				logger.Warn("message missing event_type and payload parse failed",
					slog.String("topic", topicName),
					slog.String("msg_id", msg.UUID),
					slog.String("error", err.Error()),
				)
				if ackErr := msg.Ack(); ackErr != nil {
					logger.Warn("failed to ack invalid message",
						slog.String("topic", topicName),
						slog.String("msg_id", msg.UUID),
						slog.String("error", ackErr.Error()),
					)
				}
				return nil
			}
			eventType = env.EventType
			if msg.Metadata == nil {
				msg.Metadata = map[string]string{}
			}
			msg.Metadata["event_type"] = eventType
		}

		logger.Debug("received message",
			slog.String("topic", topicName),
			slog.String("event_type", eventType),
			slog.String("msg_id", msg.UUID),
		)

		if err := c.DispatchEvent(ctx, eventType, msg.Payload); err != nil {
			logger.Error("failed to dispatch event",
				slog.String("topic", topicName),
				slog.String("event_type", eventType),
				slog.String("msg_id", msg.UUID),
				slog.String("error", err.Error()),
			)
			if nackErr := msg.Nack(); nackErr != nil {
				logger.Warn("failed to nack message",
					slog.String("topic", topicName),
					slog.String("msg_id", msg.UUID),
					slog.String("error", nackErr.Error()),
				)
			}
			return err
		}

		if ackErr := msg.Ack(); ackErr != nil {
			logger.Warn("failed to ack message",
				slog.String("topic", topicName),
				slog.String("msg_id", msg.UUID),
				slog.String("error", ackErr.Error()),
			)
			return ackErr
		}
		return nil
	}
}

package messaging

import (
	"context"
	"log/slog"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	"github.com/FangcunMount/component-base/pkg/messaging/rabbitmq"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/nsqio/go-nsq"
)

type TopicSubscriptionSource interface {
	GetTopicSubscriptions() []eventcatalog.TopicSubscription
}

type EventDispatcher interface {
	DispatchEvent(ctx context.Context, eventType string, payload []byte) error
}

type SubscriptionRuntime interface {
	TopicSubscriptionSource
	EventDispatcher
}

type MessageEventExtractor struct{}

func (MessageEventExtractor) Extract(msg *basemessaging.Message) (string, error) {
	if msg.Metadata == nil {
		msg.Metadata = map[string]string{}
	}
	if eventType, ok := msg.Metadata["event_type"]; ok {
		return eventType, nil
	}
	env, err := eventcodec.DecodeEnvelope(msg.Payload)
	if err != nil {
		return "", err
	}
	msg.Metadata["event_type"] = env.EventType
	return env.EventType, nil
}

type MessageSettlementPolicy struct {
	logger   *slog.Logger
	service  string
	topic    string
	observer eventobservability.Observer
}

func (p MessageSettlementPolicy) AckInvalid(msg *basemessaging.Message, parseErr error) {
	p.logger.Warn("message missing event_type and payload parse failed",
		slog.String("topic", p.topic),
		slog.String("msg_id", msg.UUID),
		slog.String("error", parseErr.Error()),
	)
	if ackErr := msg.Ack(); ackErr != nil {
		p.observe(msg, "", eventobservability.ConsumeOutcomePoisonAckFailed)
		p.logger.Warn("failed to ack invalid message",
			slog.String("topic", p.topic),
			slog.String("msg_id", msg.UUID),
			slog.String("error", ackErr.Error()),
		)
		return
	}
	p.observe(msg, "", eventobservability.ConsumeOutcomePoisonAcked)
}

func (p MessageSettlementPolicy) NackFailed(msg *basemessaging.Message, eventType string, dispatchErr error) eventobservability.ConsumeOutcome {
	p.logger.Error("failed to dispatch event",
		slog.String("topic", p.topic),
		slog.String("event_type", eventType),
		slog.String("msg_id", msg.UUID),
		slog.String("error", dispatchErr.Error()),
	)
	if nackErr := msg.Nack(); nackErr != nil {
		outcome := eventobservability.ConsumeOutcomeNackFailed
		p.observe(msg, eventType, outcome)
		p.logger.Warn("failed to nack message",
			slog.String("topic", p.topic),
			slog.String("msg_id", msg.UUID),
			slog.String("error", nackErr.Error()),
		)
		return outcome
	}
	outcome := eventobservability.ConsumeOutcomeNacked
	p.observe(msg, eventType, outcome)
	return outcome
}

func (p MessageSettlementPolicy) AckSuccess(msg *basemessaging.Message) (eventobservability.ConsumeOutcome, error) {
	if ackErr := msg.Ack(); ackErr != nil {
		eventType := ""
		if msg != nil && msg.Metadata != nil {
			eventType = msg.Metadata["event_type"]
		}
		outcome := eventobservability.ConsumeOutcomeAckFailed
		p.observe(msg, eventType, outcome)
		p.logger.Warn("failed to ack message",
			slog.String("topic", p.topic),
			slog.String("msg_id", msg.UUID),
			slog.String("error", ackErr.Error()),
		)
		return outcome, ackErr
	}
	eventType := ""
	if msg != nil && msg.Metadata != nil {
		eventType = msg.Metadata["event_type"]
	}
	outcome := eventobservability.ConsumeOutcomeAcked
	p.observe(msg, eventType, outcome)
	return outcome, nil
}

func (p MessageSettlementPolicy) observe(msg *basemessaging.Message, eventType string, outcome eventobservability.ConsumeOutcome) {
	if p.observer == nil {
		return
	}
	p.observer.ObserveConsume(context.Background(), eventobservability.ConsumeEvent{
		Service:   p.service,
		Topic:     p.topic,
		EventType: eventType,
		Outcome:   outcome,
	})
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

func EnsureTopics(cfg *config.MessagingConfig, logger *slog.Logger, source TopicSubscriptionSource) error {
	if source == nil {
		return nil
	}
	subscriptions := source.GetTopicSubscriptions()
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

func SubscribeHandlers(serviceName string, logger *slog.Logger, runtime SubscriptionRuntime, subscriber basemessaging.Subscriber) error {
	return SubscribeHandlersWithOptions(SubscribeHandlersOptions{
		ServiceName: serviceName,
		Logger:      logger,
		Runtime:     runtime,
		Subscriber:  subscriber,
	})
}

type SubscribeHandlersOptions struct {
	ServiceName string
	Logger      *slog.Logger
	Runtime     SubscriptionRuntime
	Subscriber  basemessaging.Subscriber
	Observer    eventobservability.Observer
}

func SubscribeHandlersWithOptions(opts SubscribeHandlersOptions) error {
	if opts.Observer == nil {
		opts.Observer = eventobservability.DefaultObserver()
	}
	serviceName := opts.ServiceName
	logger := opts.Logger
	runtime := opts.Runtime
	subscriber := opts.Subscriber
	if runtime == nil || subscriber == nil {
		return nil
	}

	subscriptions := runtime.GetTopicSubscriptions()
	for _, sub := range subscriptions {
		topicName := sub.TopicName
		msgHandler := createDispatchHandlerWithObserver(logger, runtime, topicName, serviceName, opts.Observer)
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

func createDispatchHandler(logger *slog.Logger, dispatcher EventDispatcher, topicName string) basemessaging.Handler {
	return createDispatchHandlerWithObserver(logger, dispatcher, topicName, "", eventobservability.DefaultObserver())
}

func createDispatchHandlerWithObserver(logger *slog.Logger, dispatcher EventDispatcher, topicName, serviceName string, observer eventobservability.Observer) basemessaging.Handler {
	extractor := MessageEventExtractor{}
	if observer == nil {
		observer = eventobservability.DefaultObserver()
	}
	settlement := MessageSettlementPolicy{logger: logger, service: serviceName, topic: topicName, observer: observer}
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

		startedAt := time.Now()
		if err := dispatcher.DispatchEvent(ctx, eventType, msg.Payload); err != nil {
			outcome := settlement.NackFailed(msg, eventType, err)
			eventobservability.ObserveConsumeDuration(ctx, observer, eventobservability.ConsumeDurationEvent{
				Service:   serviceName,
				Topic:     topicName,
				EventType: eventType,
				Outcome:   outcome,
				Duration:  time.Since(startedAt),
			})
			return err
		}

		outcome, err := settlement.AckSuccess(msg)
		eventobservability.ObserveConsumeDuration(ctx, observer, eventobservability.ConsumeDurationEvent{
			Service:   serviceName,
			Topic:     topicName,
			EventType: eventType,
			Outcome:   outcome,
			Duration:  time.Since(startedAt),
		})
		return err
	}
}

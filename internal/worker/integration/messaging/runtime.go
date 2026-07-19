package messaging

import (
	"context"
	"errors"
	"log/slog"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	cbrabbit "github.com/FangcunMount/component-base/pkg/messaging/rabbitmq"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/nsqio/go-nsq"
)

type TopicSubscriptionSource interface {
	GetTopicSubscriptions() []eventcatalog.TopicSubscription
}

type EventDispatcher interface {
	DispatchEvent(ctx context.Context, eventType string, payload []byte) (eventruntime.DispatchResult, error)
}

type SubscriptionRuntime interface {
	TopicSubscriptionSource
	EventDispatcher
}

type MessageEventExtractor = eventruntime.MessageEventExtractor
type MessageSettlementPolicy = eventruntime.MessageSettlementPolicy

func CreatePublisher(cfg *config.MessagingConfig) (basemessaging.Publisher, error) {
	switch cfg.Provider {
	case "rabbitmq":
		return cbrabbit.NewPublisher(cfg.RabbitMQURL)
	default:
		return cbnsq.NewPublisher(cfg.NSQAddr, nsq.NewConfig())
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
	ServiceName  string
	Logger       *slog.Logger
	Runtime      SubscriptionRuntime
	Subscriber   basemessaging.Subscriber
	Observer     eventobservability.Observer
	HoldRecorder RetryEventHoldRecorder
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
		msgHandler := createDispatchHandlerWithObserverAndHold(logger, runtime, topicName, serviceName, opts.Observer, opts.HoldRecorder)
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
	return createDispatchHandlerWithObserverAndHold(logger, dispatcher, topicName, serviceName, observer, nil)
}

func createDispatchHandlerWithObserverAndHold(logger *slog.Logger, dispatcher EventDispatcher, topicName, serviceName string, observer eventobservability.Observer, holdRecorder RetryEventHoldRecorder) basemessaging.Handler {
	extractor := MessageEventExtractor{}
	if logger == nil {
		logger = slog.Default()
	}
	if observer == nil {
		observer = eventobservability.DefaultObserver()
	}
	settlement := eventruntime.NewMessageSettlementPolicy(logger, serviceName, topicName, observer)
	return func(ctx context.Context, msg *basemessaging.Message) error {
		eventType, err := extractor.Extract(msg)
		if err != nil {
			_, settleErr := settlement.NackInvalid(msg, err)
			if settleErr != nil && !errors.Is(settleErr, err) {
				return errors.Join(err, settleErr)
			}
			return err
		}

		logLevel := dispatchLogLevel(topicName)
		logger.Log(ctx, logLevel, "dispatching event", dispatchLogFields(serviceName, topicName, eventType, msg)...)

		startedAt := time.Now()
		result, err := dispatcher.DispatchEvent(ctx, eventType, msg.Payload)
		if err != nil {
			if errors.Is(err, eventruntime.ErrAutomaticRetryPaused) {
				if holdRecorder == nil {
					settlement.NackHoldFailed(msg, eventType, errors.New("retry event hold recorder is not configured"))
					return err
				}
				if holdErr := holdRecorder.Hold(ctx, msg, eventType, err); holdErr != nil {
					settlement.NackHoldFailed(msg, eventType, holdErr)
					return errors.Join(err, holdErr)
				}
				outcome, ackErr := settlement.AckHeld(msg)
				eventobservability.ObserveConsumeDuration(ctx, observer, eventobservability.ConsumeDurationEvent{Service: serviceName, Topic: topicName, EventType: eventType, Outcome: outcome, Duration: time.Since(startedAt)})
				return ackErr
			}
			outcome := settlement.NackFailed(msg, eventType, err)
			elapsed := time.Since(startedAt)
			eventobservability.ObserveConsumeDuration(ctx, observer, eventobservability.ConsumeDurationEvent{
				Service:   serviceName,
				Topic:     topicName,
				EventType: eventType,
				Outcome:   outcome,
				Duration:  elapsed,
			})
			logger.Log(ctx, logLevel, "event dispatch settlement completed",
				append(dispatchLogFields(serviceName, topicName, eventType, msg),
					slog.String("outcome", outcome.String()),
					slog.Int64("elapsed_ms", elapsed.Milliseconds()),
				)...,
			)
			return err
		}

		var outcome eventobservability.ConsumeOutcome
		if result.Outcome == eventruntime.DispatchUnknown {
			outcome, err = settlement.AckUnknown(msg)
		} else {
			outcome, err = settlement.AckSuccess(msg)
		}
		elapsed := time.Since(startedAt)
		eventobservability.ObserveConsumeDuration(ctx, observer, eventobservability.ConsumeDurationEvent{
			Service:   serviceName,
			Topic:     topicName,
			EventType: eventType,
			Outcome:   outcome,
			Duration:  elapsed,
		})
		logger.Log(ctx, logLevel, "event dispatch completed",
			append(dispatchLogFields(serviceName, topicName, eventType, msg),
				slog.String("outcome", outcome.String()),
				slog.Int64("elapsed_ms", elapsed.Milliseconds()),
			)...,
		)
		return err
	}
}

func dispatchLogLevel(_ string) slog.Level {
	return slog.LevelDebug
}

func dispatchLogFields(serviceName, topicName, eventType string, msg *basemessaging.Message) []any {
	fields := []any{
		slog.String("channel", serviceName),
		slog.String("topic", topicName),
		slog.String("event_type", eventType),
	}
	if msg == nil {
		return append(fields, slog.Bool("message_nil", true))
	}
	return append(fields,
		slog.String("msg_id", msg.UUID),
		slog.Int("payload_bytes", len(msg.Payload)),
	)
}

package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
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

type WirePublisherCloser interface {
	WirePublisher
	Close() error
}

func CreateSDKWirePublisher(cfg *config.MessagingConfig) (WirePublisherCloser, error) {
	if cfg == nil || cfg.Provider != "nsq" {
		return nil, fmt.Errorf("native wire publisher requires NSQ provider")
	}
	return messagingruntime.NewSDKNSQWirePublisher(cfg.NSQAddr)
}

func EnsureChannels(ctx context.Context, cfg *config.MessagingConfig, serviceName string, source TopicSubscriptionSource) error {
	if source == nil {
		return fmt.Errorf("worker subscription catalog is required for NSQ channel preparation")
	}
	subscriptions := source.GetTopicSubscriptions()
	channels := make([]messagingruntime.DurableChannel, 0, len(subscriptions))
	for _, sub := range subscriptions {
		channels = append(channels, messagingruntime.DurableChannel{Topic: sub.TopicName, Channel: serviceName})
	}
	if len(channels) == 0 {
		return fmt.Errorf("worker subscription catalog has no topics to prepare")
	}
	return messagingruntime.EnsureNSQChannels(ctx, cfg.NSQDHTTPEndpoints, channels)
}

type SubscribeHandlersOptions struct {
	ServiceName     string
	Logger          *slog.Logger
	Runtime         SubscriptionRuntime
	Subscriber      basemessaging.Subscriber
	Observer        eventobservability.Observer
	HoldRecorder    DeliveryRetryEventHoldRecorder
	UnknownRecorder func(context.Context, *basemessaging.Message, string) error
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
	if opts.UnknownRecorder == nil {
		return fmt.Errorf("unknown-event recorder is required before Worker subscription")
	}

	subscriptions := runtime.GetTopicSubscriptions()
	for _, sub := range subscriptions {
		topicName := sub.TopicName
		msgHandler := createDispatchHandlerWithObserverAndHoldAndUnknown(logger, runtime, topicName, serviceName, opts.Observer, opts.HoldRecorder, opts.UnknownRecorder)
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

func createDispatchHandlerWithObserverAndHold(logger *slog.Logger, dispatcher EventDispatcher, topicName, serviceName string, observer eventobservability.Observer, holdRecorder DeliveryRetryEventHoldRecorder) basemessaging.Handler {
	return createDispatchHandlerWithObserverAndHoldAndUnknown(logger, dispatcher, topicName, serviceName, observer, holdRecorder, nil)
}

func createDispatchHandlerWithObserverAndHoldAndUnknown(logger *slog.Logger, dispatcher EventDispatcher, topicName, serviceName string, observer eventobservability.Observer, holdRecorder DeliveryRetryEventHoldRecorder, unknownRecorder func(context.Context, *basemessaging.Message, string) error) basemessaging.Handler {
	extractor := eventruntime.MessageEventExtractor{}
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
			settlement.ReportInvalid(msg, err)
			return err
		}

		logLevel := dispatchLogLevel(topicName)
		logger.Log(ctx, logLevel, "dispatching event", dispatchLogFields(serviceName, topicName, eventType, msg)...)

		startedAt := time.Now()
		result, err := dispatcher.DispatchEvent(ctx, eventType, msg.Payload)
		if err != nil {
			if errors.Is(err, eventruntime.ErrAutomaticRetryPaused) {
				if holdRecorder == nil {
					holdErr := errors.New("retry event hold recorder is not configured")
					settlement.ReportHoldFailed(msg, eventType, holdErr)
					return errors.Join(err, holdErr)
				}
				if holdErr := holdRecorder.HoldDelivery(ctx, rmtransport.Received{
					ID: msg.UUID, Topic: msg.Topic, Channel: msg.Channel,
					Payload: msg.Payload, Attempts: msg.Attempts,
				}, eventType, err); holdErr != nil {
					settlement.ReportHoldFailed(msg, eventType, holdErr)
					return errors.Join(err, holdErr)
				}
				outcome, ackErr := settlement.AckHeld(msg)
				eventobservability.ObserveConsumeDuration(ctx, observer, eventobservability.ConsumeDurationEvent{Service: serviceName, Topic: topicName, EventType: eventType, Outcome: outcome, Duration: time.Since(startedAt)})
				return ackErr
			}
			outcome := settlement.ReportFailed(msg, eventType, err)
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
			if unknownRecorder == nil {
				err = fmt.Errorf("unknown-event recorder is not configured")
			} else {
				err = unknownRecorder(ctx, msg, eventType)
			}
			if err != nil {
				outcome = settlement.ReportUnknownPersistFailed(msg, eventType, err)
				eventobservability.ObserveConsumeDuration(ctx, observer, eventobservability.ConsumeDurationEvent{Service: serviceName, Topic: topicName, EventType: eventType, Outcome: outcome, Duration: time.Since(startedAt)})
				return fmt.Errorf("persist unknown event type %q: %w", eventType, err)
			}
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

package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
)

type SDKDeliverySubscriber interface {
	Subscribe(string, string, rmtransport.Handler) error
}

type SubscribeSDKHandlersOptions struct {
	ServiceName     string
	Logger          *slog.Logger
	Runtime         SubscriptionRuntime
	Subscriber      SDKDeliverySubscriber
	Observer        eventobservability.Observer
	HoldRecorder    DeliveryRetryEventHoldRecorder
	UnknownRecorder func(context.Context, rmtransport.Received, string) error
}

func SubscribeSDKHandlersWithOptions(opts SubscribeSDKHandlersOptions) error {
	if opts.Runtime == nil || opts.Subscriber == nil {
		return errors.New("SDK Worker runtime and subscriber are required")
	}
	if opts.UnknownRecorder == nil {
		return errors.New("unknown-event recorder is required before Worker subscription")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	for _, sub := range opts.Runtime.GetTopicSubscriptions() {
		topic := sub.TopicName
		handler := createSDKDispatchHandler(logger, opts.Runtime, topic, opts.ServiceName, opts.Observer, opts.HoldRecorder, opts.UnknownRecorder)
		if err := opts.Subscriber.Subscribe(topic, opts.ServiceName, handler); err != nil {
			return fmt.Errorf("subscribe Worker SDK delivery for %s: %w", topic, err)
		}
		logger.Info("subscribed to topic", slog.String("topic", topic), slog.Int("event_count", len(sub.EventTypes)), slog.String("channel", opts.ServiceName))
	}
	return nil
}

func createSDKDispatchHandler(
	logger *slog.Logger,
	dispatcher EventDispatcher,
	topic, service string,
	observer eventobservability.Observer,
	holdRecorder DeliveryRetryEventHoldRecorder,
	unknownRecorder func(context.Context, rmtransport.Received, string) error,
) rmtransport.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if observer == nil {
		observer = eventobservability.DefaultObserver()
	}
	observe := func(ctx context.Context, msg rmtransport.Received, eventType string, outcome eventobservability.ConsumeOutcome, started time.Time) {
		attempts := max(int(msg.Attempts), 1)
		observer.ObserveConsume(ctx, eventobservability.ConsumeEvent{Service: service, Topic: topic, EventType: eventType, Outcome: outcome, Attempts: attempts})
		eventobservability.ObserveConsumeDuration(ctx, observer, eventobservability.ConsumeDurationEvent{
			Service: service, Topic: topic, EventType: eventType, Outcome: outcome, Duration: time.Since(started),
		})
	}
	ack := func(ctx context.Context, delivery rmtransport.Delivery, msg rmtransport.Received, eventType string, started time.Time, success, failure eventobservability.ConsumeOutcome) error {
		if err := delivery.Ack(); err != nil {
			observe(ctx, msg, eventType, failure, started)
			return err
		}
		observe(ctx, msg, eventType, success, started)
		return nil
	}
	return func(ctx context.Context, delivery rmtransport.Delivery) error {
		if delivery == nil {
			return errors.New("SDK delivery is nil")
		}
		msg := delivery.Message()
		started := time.Now()
		eventType := msg.Metadata["event_type"]
		if eventType == "" {
			envelope, err := domainwire.DecodeEnvelope(msg.Payload)
			if err != nil {
				observe(ctx, msg, "", eventobservability.ConsumeOutcomeDecodeFailed, started)
				return err
			}
			eventType = envelope.EventType
			if eventType == "" {
				observe(ctx, msg, "", eventobservability.ConsumeOutcomeDecodeFailed, started)
				return errors.New("event envelope has no event_type")
			}
		}
		logger.Log(ctx, dispatchLogLevel(topic), "dispatching event",
			slog.String("channel", service), slog.String("topic", topic), slog.String("event_type", eventType),
			slog.String("msg_id", msg.ID), slog.Int("payload_bytes", len(msg.Payload)))
		result, err := dispatcher.DispatchEvent(ctx, eventType, msg.Payload)
		if err != nil {
			if errors.Is(err, eventruntime.ErrAutomaticRetryPaused) {
				if holdRecorder == nil {
					holdErr := errors.New("retry event hold recorder is not configured")
					observe(ctx, msg, eventType, eventobservability.ConsumeOutcomeHoldFailed, started)
					return errors.Join(err, holdErr)
				}
				if holdErr := holdRecorder.HoldDelivery(ctx, msg, eventType, err); holdErr != nil {
					observe(ctx, msg, eventType, eventobservability.ConsumeOutcomeHoldFailed, started)
					return errors.Join(err, holdErr)
				}
				return ack(ctx, delivery, msg, eventType, started, eventobservability.ConsumeOutcomeHeld, eventobservability.ConsumeOutcomeHoldFailed)
			}
			observe(ctx, msg, eventType, eventobservability.ConsumeOutcomeDispatchFailed, started)
			return err
		}
		if result.Outcome == eventruntime.DispatchUnknown {
			if unknownRecorder == nil {
				err = errors.New("unknown-event recorder is not configured")
			} else {
				err = unknownRecorder(ctx, msg, eventType)
			}
			if err != nil {
				observe(ctx, msg, eventType, eventobservability.ConsumeOutcomeUnknownPersistFailed, started)
				return fmt.Errorf("persist unknown event type %q: %w", eventType, err)
			}
			return ack(ctx, delivery, msg, eventType, started, eventobservability.ConsumeOutcomeUnknownAcked, eventobservability.ConsumeOutcomeUnknownAckFailed)
		}
		return ack(ctx, delivery, msg, eventType, started, eventobservability.ConsumeOutcomeAcked, eventobservability.ConsumeOutcomeAckFailed)
	}
}

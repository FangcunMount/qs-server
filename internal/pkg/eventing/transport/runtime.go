package transport

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	cbrabbit "github.com/FangcunMount/component-base/pkg/messaging/rabbitmq"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/nsqio/go-nsq"
)

const (
	DefaultRetryBaseDelay = 30 * time.Second
	DefaultRetryMaxDelay  = 5 * time.Minute
	DefaultRetryJitter    = 0.2
)

type SubscriberConfig struct {
	Provider       string
	NSQLookupdAddr string
	RabbitMQURL    string
}

func NewSubscriberOptions(maxInFlight, maxAttempts int, failedHandler basemessaging.FailedMessageHandler) (basemessaging.SubscriberOptions, error) {
	if maxAttempts < 1 || maxAttempts > retrygovernance.HardMaxDeliveryAttempts {
		return basemessaging.SubscriberOptions{}, fmt.Errorf("transport max attempts must be between 1 and %d", retrygovernance.HardMaxDeliveryAttempts)
	}
	if failedHandler == nil {
		return basemessaging.SubscriberOptions{}, fmt.Errorf("transport failed-message handler is required")
	}
	return basemessaging.SubscriberOptions{
		MaxInFlight:          maxInFlight,
		MaxAttempts:          maxAttempts,
		RetryBackoff:         basemessaging.RetryBackoffOptions{BaseDelay: DefaultRetryBaseDelay, MaxDelay: DefaultRetryMaxDelay, JitterFraction: DefaultRetryJitter},
		FailedMessageHandler: failedHandler,
	}, nil
}

func NewSubscriber(config SubscriberConfig, options basemessaging.SubscriberOptions) (basemessaging.Subscriber, error) {
	if options.MaxAttempts < 1 || options.FailedMessageHandler == nil {
		return nil, fmt.Errorf("bounded transport subscriber options are required")
	}
	switch config.Provider {
	case "nsq":
		return cbnsq.NewSubscriberWithOptions([]string{config.NSQLookupdAddr}, nsq.NewConfig(), options)
	case "rabbitmq":
		return cbrabbit.NewSubscriberWithOptions(config.RabbitMQURL, options)
	default:
		return nil, fmt.Errorf("unsupported messaging provider: %s", config.Provider)
	}
}

func TerminalFailedMessageHandler(logger *slog.Logger, component string) basemessaging.FailedMessageHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, failed basemessaging.FailedMessage) error {
		messageID := ""
		if failed.Message != nil {
			messageID = failed.Message.UUID
		}
		cause := "transport delivery exhausted"
		if failed.Cause != nil {
			cause = failed.Cause.Error()
		}
		logger.Error("transport delivery exhausted without durable audit",
			slog.String("component", component), slog.String("provider", failed.Provider),
			slog.String("topic", failed.Topic), slog.String("channel", failed.Channel),
			slog.String("message_id", messageID), slog.Int("attempts", failed.Attempts), slog.String("error", cause))
		eventobservability.DefaultObserver().ObserveConsume(ctx, eventobservability.ConsumeEvent{
			Service: component, Topic: failed.Topic, Outcome: eventobservability.ConsumeOutcomeTransportTerminal,
		})
		return nil
	}
}

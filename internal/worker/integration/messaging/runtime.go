package messaging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/worker/config"
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

func dispatchLogLevel(_ string) slog.Level {
	return slog.LevelDebug
}

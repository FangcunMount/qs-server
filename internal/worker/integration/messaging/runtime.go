package messaging

import (
	"context"
	"log/slog"

	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
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

func dispatchLogLevel(_ string) slog.Level {
	return slog.LevelDebug
}

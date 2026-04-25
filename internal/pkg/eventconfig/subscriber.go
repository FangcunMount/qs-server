package eventconfig

import (
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventruntime"
)

type HandlerFunc = eventruntime.HandlerFunc
type HandlerFactory = eventruntime.HandlerFactory
type Subscriber = eventruntime.Subscriber
type TopicSubscription = eventcatalog.TopicSubscription

type SubscriberOptions struct {
	Registry       *Registry
	Catalog        *eventcatalog.Catalog
	HandlerFactory HandlerFactory
	Logger         *slog.Logger
}

func NewSubscriber(opts SubscriberOptions) *Subscriber {
	catalog := opts.Catalog
	if catalog == nil && opts.Registry != nil {
		catalog = opts.Registry.Catalog()
	}
	if catalog == nil {
		catalog = Global().Catalog()
	}
	return eventruntime.NewSubscriber(eventruntime.SubscriberOptions{
		Catalog:        catalog,
		HandlerFactory: opts.HandlerFactory,
		Logger:         opts.Logger,
	})
}

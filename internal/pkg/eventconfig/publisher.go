package eventconfig

import (
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventruntime"
)

type RoutingPublisher = eventruntime.RoutingPublisher
type PublishMode = eventruntime.PublishMode

const (
	PublishModeMQ      = eventruntime.PublishModeMQ
	PublishModeLogging = eventruntime.PublishModeLogging
	PublishModeNop     = eventruntime.PublishModeNop
)

func PublishModeFromEnv(env string) PublishMode {
	return eventruntime.PublishModeFromEnv(env)
}

type RoutingPublisherOptions struct {
	Registry      *Registry
	Catalog       *eventcatalog.Catalog
	TopicResolver eventcatalog.TopicResolver
	MQPublisher   messaging.Publisher
	Source        string
	Mode          PublishMode
}

func NewRoutingPublisher(opts RoutingPublisherOptions) *RoutingPublisher {
	resolver := opts.TopicResolver
	if resolver == nil && opts.Catalog != nil {
		resolver = opts.Catalog
	}
	if resolver == nil && opts.Registry != nil {
		resolver = opts.Registry
	}
	if resolver == nil {
		resolver = Global()
	}
	return eventruntime.NewRoutingPublisher(eventruntime.RoutingPublisherOptions{
		TopicResolver: resolver,
		MQPublisher:   opts.MQPublisher,
		Source:        opts.Source,
		Mode:          opts.Mode,
	})
}

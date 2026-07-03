package outboxruntime

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type Spec struct {
	Name                    string
	Store                   appEventing.OutboxStore
	Publisher               event.EventPublisher
	ReadyIndex              appEventing.ReadyIndex
	BatchSize               int
	PublishWorkers          int
	ImmediateMaxConcurrent  int
	ImmediateEnabled        bool
	RequireDurablePublisher bool
	BeforePublishHooks      []appEventing.OutboxBeforePublishHook
	Observer                eventobservability.Observer
}

type Runtime struct {
	Immediate              *appEventing.ImmediateDispatcher
	Relay                  appEventing.OutboxRelay
	PostCommitReadyIndexer *appEventing.PostCommitReadyIndexer
}

func Build(spec Spec) Runtime {
	if spec.Observer == nil {
		spec.Observer = eventobservability.DefaultObserver()
	}
	immediate := appEventing.NewImmediateDispatcher(appEventing.ImmediateDispatcherOptions{
		Name:          spec.Name,
		Store:         spec.Store,
		Publisher:     spec.Publisher,
		Observer:      spec.Observer,
		Enabled:       spec.ImmediateEnabled,
		MaxConcurrent: spec.ImmediateMaxConcurrent,
		Hooks:         spec.BeforePublishHooks,
		ReadyIndex:    spec.ReadyIndex,
	})
	relay := appEventing.NewOutboxRelayWithOptions(appEventing.OutboxRelayOptions{
		Name:                    spec.Name,
		Store:                   spec.Store,
		Publisher:               spec.Publisher,
		Observer:                spec.Observer,
		BatchSize:               spec.BatchSize,
		PublishWorkers:          spec.PublishWorkers,
		RequireDurablePublisher: spec.RequireDurablePublisher,
		ReadyIndex:              spec.ReadyIndex,
		BeforePublishHooks:      spec.BeforePublishHooks,
	})
	return Runtime{
		Immediate:              immediate,
		Relay:                  relay,
		PostCommitReadyIndexer: appEventing.NewPostCommitReadyIndexer(spec.ReadyIndex),
	}
}

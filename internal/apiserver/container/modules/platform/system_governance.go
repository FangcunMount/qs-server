package platform

import (
	"github.com/FangcunMount/component-base/pkg/event"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventdelivery"
	governanceinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	retrygovinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// RESTSystemGovernanceInput collects dependencies for the governance facade.
type RESTSystemGovernanceInput struct {
	Options                 *options.SystemGovernanceOptions
	EventStatusService      appEventing.StatusService
	EventOutboxes           []appEventing.NamedOutboxStatusReader
	CacheGovernance         statisticsApp.GovernanceFacade
	CachePolicyReloader     systemgov.CachePolicyReloader
	LocalResilienceSnapshot func() resilience.RuntimeSnapshot
	MySQLDB                 *gorm.DB
	MongoDB                 *mongo.Database
	ResilienceGovernor      control.Governor
	ActionAuditStore        systemgov.ActionAuditStore
	ActionHandlers          map[string]systemgov.ActionHandler
	EventPublisher          event.EventPublisher
}

// BuildRESTSystemGovernanceFacade assembles the unified governance facade.
func BuildRESTSystemGovernanceFacade(in RESTSystemGovernanceInput) systemgov.Facade {
	metrics := govprom.NewAdapter(nil)
	components := govcomponent.NewAdapter(nil)
	if in.Options != nil {
		metrics = govprom.NewAdapter(in.Options.Prometheus)
		components = govcomponent.NewAdapter(in.Options.Components)
	}
	registry := systemgov.NewActionRegistry(resilienceActionFlags(in.Options))
	auditStore := in.ActionAuditStore
	if auditStore == nil {
		auditStore = governanceinfra.NewActionAuditStore(in.MySQLDB)
	}
	retryReader := retrygovinfra.NewReader(in.MySQLDB, in.MongoDB)
	actions := systemgov.NewActionExecutorWithResilience(registry, in.CacheGovernance, in.CachePolicyReloader, in.ResilienceGovernor, auditStore).
		BindEventReplayStores(buildEventReplayStores(in.EventOutboxes, retryReader)).
		BindDeliveryReplay(eventdelivery.NewStore(in.MySQLDB), in.EventPublisher).
		BindActionHandlers(in.ActionHandlers)
	return systemgov.NewFacade(systemgov.FacadeDeps{
		EventStatusService:      in.EventStatusService,
		EventTypeSources:        buildEventTypeSources(in.EventOutboxes),
		CacheGovernance:         in.CacheGovernance,
		CachePolicyReloader:     in.CachePolicyReloader,
		LocalResilienceSnapshot: in.LocalResilienceSnapshot,
		CheckpointReader:        NewCheckpointGovernanceReader(checkpoint.NewRepository(in.MySQLDB)),
		Metrics:                 metrics,
		Components:              components,
		Registry:                registry,
		Actions:                 actions,
		RetryGovernanceReader:   retryReader,
		RetryCandidateReader:    retryReader,
	})
}

func buildEventReplayStores(outboxes []appEventing.NamedOutboxStatusReader, retryHold outboxport.ManualReplayAuthorizer) map[string]outboxport.ManualReplayAuthorizer {
	stores := map[string]outboxport.ManualReplayAuthorizer{}
	for _, outbox := range outboxes {
		if replay, ok := outbox.Reader.(outboxport.ManualReplayAuthorizer); ok && outbox.Name != "" {
			stores[outbox.Name] = replay
		}
	}
	if retryHold != nil {
		stores["retry_hold"] = retryHold
	}
	return stores
}

func resilienceActionFlags(opts *options.SystemGovernanceOptions) map[string]bool {
	flags := map[string]bool{}
	if opts == nil {
		return flags
	}
	if opts.Resilience != nil {
		flags["resilience.tune_rate_limit"] = opts.Resilience.TuneRateLimit
		flags["resilience.drain_queue"] = opts.Resilience.DrainQueue
		flags["resilience.resume_queue"] = opts.Resilience.ResumeQueue
		flags["resilience.release_lock"] = opts.Resilience.ReleaseLock
	}
	if opts.Retry != nil {
		flags["retry.manual_actions"] = opts.Retry.ManualActionsEnabled
	}
	return flags
}

func buildEventTypeSources(outboxes []appEventing.NamedOutboxStatusReader) []systemgov.EventTypeStatusSource {
	sources := make([]systemgov.EventTypeStatusSource, 0, len(outboxes))
	for _, outbox := range outboxes {
		if outbox.Reader == nil {
			continue
		}
		reader, ok := outbox.Reader.(outboxport.EventTypeStatusReader)
		if !ok {
			continue
		}
		store := outbox.Name
		if store == "" {
			store = "outbox"
		}
		sources = append(sources, systemgov.EventTypeStatusSource{
			Store:  store,
			Reader: reader,
		})
	}
	return sources
}

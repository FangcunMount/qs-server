package platform

import (
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"gorm.io/gorm"
)

// RESTSystemGovernanceInput collects dependencies for the governance facade.
type RESTSystemGovernanceInput struct {
	Options                 *options.SystemGovernanceOptions
	EventStatusService      appEventing.StatusService
	EventOutboxes           []appEventing.NamedOutboxStatusReader
	CacheGovernance         statisticsApp.GovernanceFacade
	CachePolicyReloader     systemgov.CachePolicyReloader
	LocalResilienceSnapshot func() resilienceplane.RuntimeSnapshot
	MySQLDB                 *gorm.DB
}

// BuildRESTSystemGovernanceFacade assembles the unified governance facade.
func BuildRESTSystemGovernanceFacade(in RESTSystemGovernanceInput) systemgov.Facade {
	metrics := govprom.NewAdapter(nil)
	components := govcomponent.NewAdapter(nil)
	if in.Options != nil {
		metrics = govprom.NewAdapter(in.Options.Prometheus)
		components = govcomponent.NewAdapter(in.Options.Components)
	}
	registry := systemgov.NewActionRegistry()
	return systemgov.NewFacade(systemgov.FacadeDeps{
		EventStatusService:      in.EventStatusService,
		EventTypeSources:        buildEventTypeSources(in.EventOutboxes),
		CacheGovernance:         in.CacheGovernance,
		CachePolicyReloader:     in.CachePolicyReloader,
		LocalResilienceSnapshot: in.LocalResilienceSnapshot,
		CheckpointReader:        NewCheckpointGovernanceReader(checkpoint.NewRepository(in.MySQLDB)),
		Metrics:                 metrics,
		Components:              components,
		Actions:                 systemgov.NewActionExecutor(registry, in.CacheGovernance, in.CachePolicyReloader),
	})
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

// BuildLocalResilienceSnapshot mirrors the apiserver /resilience/status assembly.
func BuildLocalResilienceSnapshot(component string, rateEnabled bool, backpressure []resilienceplane.BackpressureSnapshot) func() resilienceplane.RuntimeSnapshot {
	return func() resilienceplane.RuntimeSnapshot {
		snapshot := resilienceplane.NewRuntimeSnapshot(component, time.Now())
		snapshot.RateLimits = []resilienceplane.CapabilitySnapshot{
			{Name: "rest_global", Kind: resilienceplane.ProtectionRateLimit.String(), Strategy: "local", Configured: rateEnabled},
			{Name: "rest_user", Kind: resilienceplane.ProtectionRateLimit.String(), Strategy: "local_key", Configured: rateEnabled},
		}
		snapshot.Backpressure = append(snapshot.Backpressure, backpressure...)
		snapshot.Locks = []resilienceplane.CapabilitySnapshot{
			{Name: "plan_scheduler_leader", Kind: resilienceplane.ProtectionLock.String(), Strategy: "redis_lock", Configured: true},
			{Name: "statistics_sync_leader", Kind: resilienceplane.ProtectionLock.String(), Strategy: "redis_lock", Configured: true},
			{Name: "behavior_pending_reconcile", Kind: resilienceplane.ProtectionLock.String(), Strategy: "redis_lock", Configured: true},
		}
		return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
	}
}

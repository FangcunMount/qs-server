package platform

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	governanceinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
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
	ResilienceGovernor      control.Governor
	ActionAuditStore        systemgov.ActionAuditStore
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
		Actions:                 systemgov.NewActionExecutorWithResilience(registry, in.CacheGovernance, in.CachePolicyReloader, in.ResilienceGovernor, auditStore),
	})
}

func resilienceActionFlags(opts *options.SystemGovernanceOptions) map[string]bool {
	flags := map[string]bool{}
	if opts == nil || opts.Resilience == nil {
		return flags
	}
	flags["resilience.tune_rate_limit"] = opts.Resilience.TuneRateLimit
	flags["resilience.drain_queue"] = opts.Resilience.DrainQueue
	flags["resilience.resume_queue"] = opts.Resilience.ResumeQueue
	flags["resilience.release_lock"] = opts.Resilience.ReleaseLock
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

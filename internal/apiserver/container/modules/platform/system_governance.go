package platform

import (
	"context"
	"errors"

	"github.com/FangcunMount/component-base/pkg/event"
	cachegovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventdelivery"
	governanceinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	retrygovinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
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
	CacheGovernance         cachegovernance.Facade
	CachePolicyReloader     systemgov.CachePolicyReloader
	LocalResilienceSnapshot func() resilience.RuntimeSnapshot
	MySQLDB                 *gorm.DB
	MongoDB                 *mongo.Database
	ResilienceGovernor      control.Governor
	ActionAuditStore        systemgov.ActionAuditStore
	ActionHandlers          map[string]systemgov.ActionHandler
	EventPublisher          event.EventPublisher
	ReportResolutionReader  interpretationreadmodel.ReportResolutionReader
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
	durableReplays, replayResolvers := buildDurableEventReplayStores(in.EventOutboxes)
	if len(durableReplays) > 0 {
		if in.MySQLDB == nil {
			// The standard replay ledger cannot run without a persistent
			// governance audit, even when its Outbox lives in MongoDB.
			auditStore = unavailableReplayAuditStore{}
		} else {
			auditStore = systemgov.NewReconcilingActionAuditStore(
				auditStore, governanceinfra.NewActionAuditStore(in.MySQLDB), replayResolvers)
		}
	}
	retryReader := retrygovinfra.NewReader(in.MySQLDB, in.MongoDB).
		WithStandardOutboxes(buildStandardOutboxGovernanceReaders(in.EventOutboxes))
	actions := systemgov.NewActionExecutorWithResilience(registry, in.CacheGovernance, in.CachePolicyReloader, in.ResilienceGovernor, auditStore).
		BindEventReplayStores(buildEventReplayStores(in.EventOutboxes, retryReader)).
		BindDurableEventReplayStores(durableReplays).
		BindDeliveryReplay(eventdelivery.NewStore(in.MySQLDB), in.EventPublisher).
		BindActionHandlers(in.ActionHandlers)
	var pendingReplayAuditReader systemgov.PendingReplayAuditReader
	var deliveryReplayReviewReader systemgov.DeliveryReplayReviewReader
	var deliveryResolver systemgov.DeliveryResolver
	if in.MySQLDB != nil {
		reader := governanceinfra.NewActionAuditStore(in.MySQLDB)
		pendingReplayAuditReader = reader
		deliveryReplayReviewReader = reader
		if replay, found := registry.Get("events.replay_delivery"); found && replay.Enabled {
			deliveryResolver = governanceinfra.NewReportGeneratedDeliveryResolver(in.MySQLDB, in.ReportResolutionReader)
		}
	}
	return systemgov.NewFacade(systemgov.FacadeDeps{
		EventStatusService:         in.EventStatusService,
		EventTypeSources:           buildEventTypeSources(in.EventOutboxes),
		CacheGovernance:            in.CacheGovernance,
		CachePolicyReloader:        in.CachePolicyReloader,
		LocalResilienceSnapshot:    in.LocalResilienceSnapshot,
		CheckpointReader:           NewCheckpointGovernanceReader(checkpoint.NewRepository(in.MySQLDB)),
		Metrics:                    metrics,
		Components:                 components,
		Registry:                   registry,
		Actions:                    actions,
		RetryGovernanceReader:      retryReader,
		RetryCandidateReader:       retryReader,
		PendingReplayAuditReader:   pendingReplayAuditReader,
		DeliveryReplayReviewReader: deliveryReplayReviewReader,
		DeliveryResolver:           deliveryResolver,
	})
}

func buildStandardOutboxGovernanceReaders(outboxes []appEventing.NamedOutboxStatusReader) map[string]systemgov.OutboxGovernanceReader {
	readers := map[string]systemgov.OutboxGovernanceReader{}
	for _, outbox := range outboxes {
		if outbox.Name == "" || outbox.Reader == nil {
			continue
		}
		if reader, ok := outbox.Reader.(systemgov.OutboxGovernanceReader); ok {
			readers[outbox.Name] = reader
		}
	}
	return readers
}

// A durable owner without a recovery resolver is not executable. A committed
// authorization must always be recoverable by its original request identity.
func buildDurableEventReplayStores(outboxes []appEventing.NamedOutboxStatusReader) (map[string]outboxport.DurableManualReplayAuthorizer, map[string]systemgov.PendingReplayResolver) {
	stores := map[string]outboxport.DurableManualReplayAuthorizer{}
	resolvers := map[string]systemgov.PendingReplayResolver{}
	for _, outbox := range outboxes {
		if outbox.Name == "" || outbox.Reader == nil {
			continue
		}
		replay, hasReplay := outbox.Reader.(outboxport.DurableManualReplayAuthorizer)
		resolver, hasResolver := outbox.Reader.(systemgov.PendingReplayResolver)
		if hasReplay && hasResolver {
			stores[outbox.Name], resolvers[outbox.Name] = replay, resolver
		}
	}
	return stores, resolvers
}

type unavailableReplayAuditStore struct{}

func (unavailableReplayAuditStore) Claim(context.Context, systemgov.ActionAuditRecord) (*systemgov.ActionAuditReplay, bool, error) {
	return nil, false, errors.New("standard replay requires a persistent MySQL governance audit")
}

func (unavailableReplayAuditStore) Complete(context.Context, systemgov.ActionAuditRecord) error {
	return errors.New("standard replay requires a persistent MySQL governance audit")
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

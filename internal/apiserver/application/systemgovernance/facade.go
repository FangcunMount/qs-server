package systemgovernance

import (
	"context"
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// Facade is the unified system governance entry point.
type Facade interface {
	GetOverview(ctx context.Context, window string) (*OverviewResponse, error)
	GetEvents(ctx context.Context, window string) (*EventsView, error)
	GetCache(ctx context.Context, window string) (*CacheView, error)
	GetResilience(ctx context.Context, window string) (*ResilienceView, error)
	ListActions(ctx context.Context) (*ActionsView, error)
	RunAction(ctx context.Context, orgID int64, actionID string, req ActionRunRequest) (*ActionRunResult, error)
}

// FacadeDeps wires governance data sources.
type FacadeDeps struct {
	EventStatusService      appEventing.StatusService
	EventTypeSources        []EventTypeStatusSource
	CacheGovernance         statisticsApp.GovernanceFacade
	LocalResilienceSnapshot func() resilienceplane.RuntimeSnapshot
	Metrics                 *govprom.Adapter
	Components              *govcomponent.Adapter
	Actions                 *ActionExecutor
}

type facade struct {
	deps      FacadeDeps
	evaluator *Evaluator
	registry  *ActionRegistry
	now       func() time.Time
}

// NewFacade creates the governance facade.
func NewFacade(deps FacadeDeps) Facade {
	registry := NewActionRegistry()
	if deps.Actions == nil {
		deps.Actions = NewActionExecutor(registry, deps.CacheGovernance)
	}
	return &facade{
		deps:      deps,
		evaluator: NewEvaluator(deps.Metrics),
		registry:  registry,
		now:       time.Now,
	}
}

func (f *facade) GetOverview(ctx context.Context, window string) (*OverviewResponse, error) {
	_, windowLabel, evalAt, err := f.resolveWindow(window)
	if err != nil {
		return nil, err
	}
	events, err := f.GetEvents(ctx, windowLabel)
	if err != nil {
		return nil, err
	}
	cache, err := f.GetCache(ctx, windowLabel)
	if err != nil {
		return nil, err
	}
	resilience, err := f.GetResilience(ctx, windowLabel)
	if err != nil {
		return nil, err
	}
	allSignals := append(append(events.Signals, cache.Signals...), resilience.Signals...)
	return &OverviewResponse{
		GeneratedAt:     evalAt,
		Window:          windowLabel,
		OverallSeverity: OverallSeverity(allSignals),
		Metrics:         f.metricsSummary(ctx, windowLabel, evalAt),
		Signals:         SortSignals(allSignals),
		Domains:         DomainSummaries(allSignals),
	}, nil
}

func (f *facade) GetEvents(ctx context.Context, window string) (*EventsView, error) {
	_, windowLabel, evalAt, err := f.resolveWindow(window)
	if err != nil {
		return nil, err
	}
	var snapshot *appEventing.StatusSnapshot
	if f.deps.EventStatusService != nil {
		snapshot, err = f.deps.EventStatusService.GetStatus(ctx)
		if err != nil {
			return nil, err
		}
	}
	eventTypes := ReadEventTypes(ctx, f.deps.EventTypeSources, evalAt)
	return &EventsView{
		GeneratedAt: evalAt,
		Window:      windowLabel,
		Metrics:     f.metricsSummary(ctx, windowLabel, evalAt),
		Signals:     f.evaluator.EvaluateEvents(ctx, snapshot, eventTypes, windowLabel, evalAt),
		Snapshot:    snapshot,
		EventTypes:  eventTypes,
	}, nil
}

func (f *facade) GetCache(ctx context.Context, window string) (*CacheView, error) {
	_, windowLabel, evalAt, err := f.resolveWindow(window)
	if err != nil {
		return nil, err
	}
	var snapshot *cachegov.StatusSnapshot
	if f.deps.CacheGovernance != nil {
		snapshot, err = f.deps.CacheGovernance.GetStatus(ctx)
		if err != nil {
			return nil, err
		}
	}
	return &CacheView{
		GeneratedAt: evalAt,
		Window:      windowLabel,
		Metrics:     f.metricsSummary(ctx, windowLabel, evalAt),
		Signals:     f.evaluator.EvaluateCache(ctx, snapshot, windowLabel, evalAt),
		Snapshot:    snapshot,
	}, nil
}

func (f *facade) GetResilience(ctx context.Context, window string) (*ResilienceView, error) {
	_, windowLabel, evalAt, err := f.resolveWindow(window)
	if err != nil {
		return nil, err
	}
	local := resilienceplane.NewRuntimeSnapshot("apiserver", evalAt)
	if f.deps.LocalResilienceSnapshot != nil {
		local = f.deps.LocalResilienceSnapshot()
	}
	remoteFetched := map[string]govcomponent.ResilienceResult{}
	if f.deps.Components != nil {
		remoteFetched = f.deps.Components.FetchResilience(ctx)
	}
	remote := make(map[string]ComponentResilience, len(remoteFetched))
	for name, item := range remoteFetched {
		remote[name] = ComponentResilience(item)
	}
	components := map[string]ComponentResilience{
		"apiserver": {Available: true, Snapshot: &local},
	}
	for name, item := range remote {
		components[name] = item
	}
	return &ResilienceView{
		GeneratedAt: evalAt,
		Window:      windowLabel,
		Metrics:     f.metricsSummary(ctx, windowLabel, evalAt),
		Signals:     f.evaluator.EvaluateResilience(ctx, local, remote, windowLabel, evalAt),
		Components:  components,
	}, nil
}

func (f *facade) ListActions(ctx context.Context) (*ActionsView, error) {
	_ = ctx
	now := time.Now()
	if f != nil && f.now != nil {
		now = f.now()
	}
	return &ActionsView{
		GeneratedAt: now,
		Actions:     f.registry.List(),
	}, nil
}

func (f *facade) RunAction(ctx context.Context, orgID int64, actionID string, req ActionRunRequest) (*ActionRunResult, error) {
	if f == nil || f.deps.Actions == nil {
		return nil, errActionsUnavailable()
	}
	return f.deps.Actions.Run(ctx, orgID, actionID, req)
}

func (f *facade) resolveWindow(window string) (time.Duration, string, time.Time, error) {
	duration, label, err := ParseWindow(window)
	if err != nil {
		return 0, "", time.Time{}, err
	}
	now := time.Now()
	if f != nil && f.now != nil {
		now = f.now()
	}
	_ = duration
	return duration, label, now, nil
}

func (f *facade) metricsSummary(ctx context.Context, window string, evalAt time.Time) MetricsSummary {
	if f == nil || f.deps.Metrics == nil {
		return MetricsSummary{Available: false, Reason: "prometheus not configured"}
	}
	probe := f.deps.Metrics.Probe(ctx, evalAt)
	return MetricsSummary{Available: probe.Available, Reason: probe.Reason}
}

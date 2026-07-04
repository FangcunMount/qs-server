package systemgovernance

import (
	"context"
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
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

// MetricsClient provides Prometheus availability and query evidence.
type MetricsClient interface {
	MetricsReader
	Probe(ctx context.Context, evalAt time.Time) govprom.Summary
}

// FacadeDeps wires governance data sources.
type FacadeDeps struct {
	EventStatusService      appEventing.StatusService
	EventTypeSources        []EventTypeStatusSource
	CacheGovernance         statisticsApp.GovernanceFacade
	LocalResilienceSnapshot func() resilienceplane.RuntimeSnapshot
	Metrics                 MetricsClient
	Components              *govcomponent.Adapter
	Actions                 *ActionExecutor
}

type facade struct {
	deps      FacadeDeps
	evaluator *Evaluator
	registry  *ActionRegistry
	now       func() time.Time
}

type evaluationContext struct {
	windowLabel string
	evalAt      time.Time
	metrics     MetricsSummary
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
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	events, err := f.collectEvents(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	cache, err := f.collectCache(ctx, evalCtx, false)
	if err != nil {
		return nil, err
	}
	resilience, err := f.collectResilience(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	allSignals := append(append(events.Signals, cache.Signals...), resilience.Signals...)
	return &OverviewResponse{
		GeneratedAt:     evalCtx.evalAt,
		Window:          evalCtx.windowLabel,
		OverallSeverity: OverallSeverity(allSignals),
		Metrics:         evalCtx.metrics,
		Signals:         SortSignals(allSignals),
		Domains:         DomainSummaries(allSignals),
	}, nil
}

func (f *facade) GetEvents(ctx context.Context, window string) (*EventsView, error) {
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	return f.collectEvents(ctx, evalCtx)
}

func (f *facade) collectEvents(ctx context.Context, evalCtx evaluationContext) (*EventsView, error) {
	var snapshot *appEventing.StatusSnapshot
	var err error
	if f.deps.EventStatusService != nil {
		snapshot, err = f.deps.EventStatusService.GetStatus(ctx)
		if err != nil {
			return nil, err
		}
	}
	eventTypes := ReadEventTypes(ctx, f.deps.EventTypeSources, evalCtx.evalAt)
	projection := NewEventDrainEvaluator(f.deps.Metrics).Evaluate(ctx, snapshot, eventTypes, evalCtx.windowLabel, evalCtx.evalAt)
	return &EventsView{
		GeneratedAt: evalCtx.evalAt,
		Window:      evalCtx.windowLabel,
		Metrics:     evalCtx.metrics,
		Signals:     projection.Signals,
		Snapshot:    snapshot,
		EventTypes:  eventTypes,
		Summary:     projection.Summary,
		OutboxRows:  projection.OutboxRows,
		TypeRows:    projection.EventTypeRows,
	}, nil
}

func (f *facade) GetCache(ctx context.Context, window string) (*CacheView, error) {
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	return f.collectCache(ctx, evalCtx, true)
}

func (f *facade) collectCache(ctx context.Context, evalCtx evaluationContext, includeHotsets bool) (*CacheView, error) {
	var snapshot *cachegov.StatusSnapshot
	var err error
	if f.deps.CacheGovernance != nil {
		snapshot, err = f.deps.CacheGovernance.GetStatus(ctx)
		if err != nil {
			return nil, err
		}
	}
	components := f.collectCacheComponents(ctx, snapshot)
	var hotsets []CacheHotsetView
	if includeHotsets {
		hotsets = f.collectCacheHotsets(ctx)
	}
	evaluator := NewCacheWarmupEvaluator(f.deps.Metrics)
	projection := evaluator.Evaluate(ctx, components, hotsets, evalCtx.windowLabel, evalCtx.evalAt)
	if snapshot != nil && len(snapshot.Warmup.LatestRuns) > 0 {
		latest := snapshot.Warmup.LatestRuns[0]
		projection.Signals = append(projection.Signals, evaluator.WarmupSignals(ctx, observabilityWarmupLatestRun{
			Trigger:     latest.Trigger,
			ErrorCount:  latest.ErrorCount,
			TargetCount: latest.TargetCount,
		}, evalCtx.windowLabel, evalCtx.evalAt)...)
		projection.Signals = SortSignals(projection.Signals)
	}
	return &CacheView{
		GeneratedAt: evalCtx.evalAt,
		Window:      evalCtx.windowLabel,
		Metrics:     evalCtx.metrics,
		Signals:     projection.Signals,
		Snapshot:    snapshot,
		Components:  components,
		FamilyRows:  projection.FamilyRows,
		WarmupKinds: projection.WarmupKinds,
		Hotsets:     projection.Hotsets,
	}, nil
}

func (f *facade) collectCacheComponents(ctx context.Context, snapshot *cachegov.StatusSnapshot) map[string]ComponentCache {
	components := map[string]ComponentCache{}
	if snapshot != nil {
		runtimeSnapshot := snapshot.RuntimeSnapshot
		name := nonEmpty(runtimeSnapshot.Component, "apiserver")
		components[name] = ComponentCache{Available: true, Snapshot: &runtimeSnapshot}
	}
	if f != nil && f.deps.Components != nil {
		for name, item := range f.deps.Components.FetchCache(ctx) {
			components[name] = ComponentCache(item)
		}
	}
	return components
}

func (f *facade) collectCacheHotsets(ctx context.Context) []CacheHotsetView {
	if f == nil || f.deps.CacheGovernance == nil {
		return nil
	}
	kinds := []cachetarget.WarmupKind{
		cachetarget.WarmupKindStaticScale,
		cachetarget.WarmupKindStaticQuestionnaire,
		cachetarget.WarmupKindStaticScaleList,
		cachetarget.WarmupKindStaticPersonalityModel,
		cachetarget.WarmupKindQueryStatsOverview,
		cachetarget.WarmupKindQueryStatsSystem,
		cachetarget.WarmupKindQueryStatsQuestionnaire,
		cachetarget.WarmupKindQueryStatsPlan,
	}
	hotsets := make([]CacheHotsetView, 0, len(kinds))
	for _, kind := range kinds {
		result, err := f.deps.CacheGovernance.GetHotset(ctx, string(kind), "5")
		hotsets = append(hotsets, CacheHotsetViewFromResponse(kind, result, err))
	}
	return hotsets
}

func (f *facade) GetResilience(ctx context.Context, window string) (*ResilienceView, error) {
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	return f.collectResilience(ctx, evalCtx)
}

func (f *facade) collectResilience(ctx context.Context, evalCtx evaluationContext) (*ResilienceView, error) {
	local := resilienceplane.NewRuntimeSnapshot("apiserver", evalCtx.evalAt)
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
	projection := NewResilienceProjector(f.deps.Metrics).Evaluate(ctx, components, evalCtx.windowLabel, evalCtx.evalAt)
	return &ResilienceView{
		GeneratedAt:      evalCtx.evalAt,
		Window:           evalCtx.windowLabel,
		Metrics:          evalCtx.metrics,
		Signals:          projection.Signals,
		Components:       components,
		Summary:          projection.Summary,
		QueueRows:        projection.QueueRows,
		BackpressureRows: projection.BackpressureRows,
		CapabilityRows:   projection.CapabilityRows,
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

func (f *facade) newEvaluationContext(ctx context.Context, window string) (evaluationContext, error) {
	_, windowLabel, evalAt, err := f.resolveWindow(window)
	if err != nil {
		return evaluationContext{}, err
	}
	return evaluationContext{
		windowLabel: windowLabel,
		evalAt:      evalAt,
		metrics:     f.metricsSummary(ctx, evalAt),
	}, nil
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

func (f *facade) metricsSummary(ctx context.Context, evalAt time.Time) MetricsSummary {
	if f == nil || f.deps.Metrics == nil {
		return MetricsSummary{Available: false, Reason: "prometheus not configured"}
	}
	probe := f.deps.Metrics.Probe(ctx, evalAt)
	return MetricsSummary{Available: probe.Available, Reason: probe.Reason}
}

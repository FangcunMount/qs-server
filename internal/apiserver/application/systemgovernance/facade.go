package systemgovernance

import (
	"context"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

// Facade 是unified system governance entry point。
type Facade interface {
	GetOverview(ctx context.Context, window string) (*OverviewResponse, error)
	GetEvents(ctx context.Context, window string) (*EventsView, error)
	GetCache(ctx context.Context, window string) (*CacheView, error)
	GetResilience(ctx context.Context, window string) (*ResilienceView, error)
	GetCheckpoints(ctx context.Context, window string) (*CheckpointView, error)
	ListActions(ctx context.Context) (*ActionsView, error)
	RunAction(ctx context.Context, orgID int64, actionID string, req ActionRunRequest) (*ActionRunResult, error)
}

// MetricsClient 提供Prometheus availability 和 查询 evidence。
type MetricsClient interface {
	MetricsReader
	Probe(ctx context.Context, evalAt time.Time) govprom.Summary
}

// FacadeDeps 线缆s 治理数据源。
type FacadeDeps struct {
	EventStatusService      appEventing.StatusService
	EventTypeSources        []EventTypeStatusSource
	CacheGovernance         statisticsApp.GovernanceFacade
	LocalResilienceSnapshot func() resilience.RuntimeSnapshot
	CheckpointReader        CheckpointStatusReader
	Metrics                 MetricsClient
	Components              *govcomponent.Adapter
	Actions                 *ActionExecutor
	Registry                *ActionRegistry
	CachePolicyReloader     CachePolicyReloader
}

type facade struct {
	deps     FacadeDeps
	registry *ActionRegistry
	now      func() time.Time
}

type evaluationContext struct {
	windowLabel string
	evalAt      time.Time
	metrics     MetricsSummary
}

// NewFacade 创建governance 门面。
func NewFacade(deps FacadeDeps) Facade {
	registry := deps.Registry
	if registry == nil && deps.Actions != nil {
		registry = deps.Actions.registry
	}
	if registry == nil {
		registry = NewActionRegistry()
	}
	if deps.Actions == nil {
		deps.Actions = NewActionExecutor(registry, deps.CacheGovernance, deps.CachePolicyReloader)
	}
	return &facade{
		deps:     deps,
		registry: registry,
		now:      time.Now,
	}
}

func (f *facade) GetOverview(ctx context.Context, window string) (*OverviewResponse, error) {
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	events, err := f.eventCollector().Collect(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	cache, err := f.cacheCollector().Collect(ctx, evalCtx, false)
	if err != nil {
		return nil, err
	}
	resilience, err := f.resilienceCollector().Collect(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	checkpoints, err := f.checkpointCollector().Collect(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	allSignals := append(append(append(events.Signals, cache.Signals...), resilience.Signals...), checkpoints.Signals...)
	return &OverviewResponse{
		GeneratedAt:     evalCtx.evalAt,
		Window:          evalCtx.windowLabel,
		OverallSeverity: OverallSeverity(allSignals),
		Metrics:         evalCtx.metrics,
		Signals:         SortSignals(allSignals),
		Domains:         DomainSummaries(allSignals),
		Checkpoints:     checkpoints,
	}, nil
}

func (f *facade) GetEvents(ctx context.Context, window string) (*EventsView, error) {
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	return f.eventCollector().Collect(ctx, evalCtx)
}

func (f *facade) GetCache(ctx context.Context, window string) (*CacheView, error) {
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	return f.cacheCollector().Collect(ctx, evalCtx, true)
}

func (f *facade) GetResilience(ctx context.Context, window string) (*ResilienceView, error) {
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	return f.resilienceCollector().Collect(ctx, evalCtx)
}

func (f *facade) GetCheckpoints(ctx context.Context, window string) (*CheckpointView, error) {
	evalCtx, err := f.newEvaluationContext(ctx, window)
	if err != nil {
		return nil, err
	}
	return f.checkpointCollector().Collect(ctx, evalCtx)
}

func (f *facade) eventCollector() eventGovernanceCollector {
	return eventGovernanceCollector{
		statusService: f.deps.EventStatusService,
		typeSources:   f.deps.EventTypeSources,
		metrics:       f.deps.Metrics,
	}
}

func (f *facade) cacheCollector() cacheGovernanceCollector {
	return cacheGovernanceCollector{
		governance: f.deps.CacheGovernance,
		components: f.deps.Components,
		metrics:    f.deps.Metrics,
	}
}

func (f *facade) resilienceCollector() resilienceGovernanceCollector {
	return resilienceGovernanceCollector{
		localSnapshot: f.deps.LocalResilienceSnapshot,
		components:    f.deps.Components,
		metrics:       f.deps.Metrics,
	}
}

func (f *facade) checkpointCollector() checkpointGovernanceCollector {
	return checkpointGovernanceCollector{reader: f.deps.CheckpointReader}
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

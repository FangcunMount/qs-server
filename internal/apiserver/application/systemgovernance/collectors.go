package systemgovernance

import (
	"context"

	cachegovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

type eventGovernanceCollector struct {
	statusService appEventing.StatusService
	typeSources   []EventTypeStatusSource
	metrics       MetricsReader
	retry         RetryGovernanceReader
}

func (c eventGovernanceCollector) Collect(ctx context.Context, evalCtx evaluationContext, orgID int64) (*EventsView, error) {
	var snapshot *appEventing.StatusSnapshot
	var err error
	if c.statusService != nil {
		snapshot, err = c.statusService.GetStatus(ctx)
		if err != nil {
			return nil, err
		}
	}
	eventTypes := ReadEventTypes(ctx, c.typeSources, evalCtx.evalAt)
	projection := NewEventDrainEvaluator(c.metrics).Evaluate(ctx, snapshot, eventTypes, evalCtx.windowLabel, evalCtx.evalAt)
	retrySummary := RetryGovernanceSummary{}
	if c.retry != nil && orgID > 0 {
		retrySummary, err = c.retry.ReadRetryGovernance(ctx, orgID)
		if err != nil {
			return nil, err
		}
	}
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
		Retry:       retrySummary,
	}, nil
}

type cacheGovernanceCollector struct {
	governance cachegovernance.Facade
	components *govcomponent.Adapter
	metrics    MetricsReader
}

func (c cacheGovernanceCollector) Collect(ctx context.Context, evalCtx evaluationContext, includeHotsets bool) (*CacheView, error) {
	var snapshot *cachemodel.StatusSnapshot
	var err error
	if c.governance != nil {
		snapshot, err = c.governance.GetStatus(ctx)
		if err != nil {
			return nil, err
		}
	}
	components := c.collectComponents(ctx, snapshot)
	var hotsets []CacheHotsetView
	if includeHotsets {
		hotsets = c.collectHotsets(ctx)
	}
	evaluator := NewCacheWarmupEvaluator(c.metrics)
	projection := evaluator.EvaluateWithLatestRun(ctx, components, hotsets, latestWarmupRun(snapshot), evalCtx.windowLabel, evalCtx.evalAt)
	var capabilityRows []CacheCapabilityRow
	if snapshot != nil {
		capabilityRows = evaluator.CapabilityRows(ctx, snapshot.EffectiveRegistry, evalCtx.windowLabel, evalCtx.evalAt)
	}
	return &CacheView{
		GeneratedAt:    evalCtx.evalAt,
		Window:         evalCtx.windowLabel,
		Metrics:        evalCtx.metrics,
		Signals:        projection.Signals,
		Snapshot:       snapshot,
		Components:     components,
		FamilyRows:     projection.FamilyRows,
		CapabilityRows: capabilityRows,
		WarmupKinds:    projection.WarmupKinds,
		Hotsets:        projection.Hotsets,
	}, nil
}

func (c cacheGovernanceCollector) collectComponents(ctx context.Context, snapshot *cachemodel.StatusSnapshot) map[string]ComponentCache {
	components := map[string]ComponentCache{}
	if snapshot != nil {
		runtimeSnapshot := projectRedisRuntimeSnapshot(snapshot.RuntimeSnapshot)
		name := nonEmpty(runtimeSnapshot.Component, "apiserver")
		components[name] = ComponentCache{Available: true, Snapshot: &runtimeSnapshot}
	}
	if c.components != nil {
		for name, item := range c.components.FetchCache(ctx) {
			components[name] = ComponentCache(item)
		}
	}
	return components
}

func (c cacheGovernanceCollector) collectHotsets(ctx context.Context) []CacheHotsetView {
	if c.governance == nil {
		return nil
	}
	kinds := DefaultCacheWarmupKinds()
	hotsets := make([]CacheHotsetView, 0, len(kinds))
	for _, descriptor := range kinds {
		result, err := c.governance.GetHotset(ctx, string(descriptor.Kind), "5")
		hotsets = append(hotsets, CacheHotsetViewFromResponse(descriptor.Kind, result, err))
	}
	return hotsets
}

func latestWarmupRun(snapshot *cachemodel.StatusSnapshot) *observabilityWarmupLatestRun {
	if snapshot == nil || len(snapshot.Warmup.LatestRuns) == 0 {
		return nil
	}
	latest := snapshot.Warmup.LatestRuns[0]
	return &observabilityWarmupLatestRun{
		Trigger:     latest.Trigger,
		ErrorCount:  latest.ErrorCount,
		TargetCount: latest.TargetCount,
	}
}

func projectRedisRuntimeSnapshot(in cachemodel.RuntimeSnapshot) observability.RuntimeSnapshot {
	out := observability.RuntimeSnapshot{
		GeneratedAt: in.GeneratedAt,
		Component:   in.Component,
		InstanceID:  in.InstanceID,
		Generation:  in.Generation,
		Summary: observability.RuntimeSummary{
			FamilyTotal: in.Summary.FamilyTotal, AvailableCount: in.Summary.AvailableCount,
			DegradedCount: in.Summary.DegradedCount, UnavailableCount: in.Summary.UnavailableCount,
			Ready: in.Summary.Ready,
		},
		Families: make([]observability.FamilyStatus, 0, len(in.Families)),
	}
	for _, family := range in.Families {
		out.Families = append(out.Families, observability.FamilyStatus{
			Component: family.Component, Family: family.Family, Profile: family.Profile,
			Namespace: family.Namespace, AllowWarmup: family.AllowWarmup,
			Configured: family.Configured, Available: family.Available, Degraded: family.Degraded,
			Mode: family.Mode, LastError: family.LastError, LastSuccessAt: family.LastSuccessAt,
			LastFailureAt: family.LastFailureAt, ConsecutiveFailures: family.ConsecutiveFailures,
			UpdatedAt: family.UpdatedAt,
		})
	}
	return out
}

type resilienceGovernanceCollector struct {
	localSnapshot func() resilience.RuntimeSnapshot
	components    *govcomponent.Adapter
	metrics       MetricsReader
}

func (c resilienceGovernanceCollector) Collect(ctx context.Context, evalCtx evaluationContext) (*ResilienceView, error) {
	local := resilience.NewRuntimeSnapshot("apiserver", evalCtx.evalAt)
	if c.localSnapshot != nil {
		local = c.localSnapshot()
	}
	remoteFetched := map[string]govcomponent.ResilienceResult{}
	if c.components != nil {
		remoteFetched = c.components.FetchResilience(ctx)
	}
	components := map[string]ComponentResilience{
		"apiserver": {Available: true, Snapshot: &local},
	}
	for name, item := range remoteFetched {
		components[name] = ComponentResilience(item)
	}
	projection := NewResilienceProjector(c.metrics).Evaluate(ctx, components, evalCtx.windowLabel, evalCtx.evalAt)
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

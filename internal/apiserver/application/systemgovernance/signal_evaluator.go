package systemgovernance

import (
	"context"
	"sort"
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

const pendingOldestAgeWarning = 5 * time.Minute

// MetricsReader loads near-window Prometheus metrics.
type MetricsReader interface {
	Query(ctx context.Context, spec govprom.QuerySpec, evalAt time.Time) govprom.MetricResult
}

// Evaluator generates bounded governance signals across domains.
type Evaluator struct {
	metrics MetricsReader
}

// NewEvaluator creates a signal evaluator.
func NewEvaluator(metrics MetricsReader) *Evaluator {
	return &Evaluator{metrics: metrics}
}

func toMetricEvidence(item govprom.MetricResult) MetricEvidence {
	return MetricEvidence{
		Name:      item.Name,
		Window:    item.Window,
		Value:     item.Value,
		Unit:      item.Unit,
		Available: item.Available,
		Reason:    item.Reason,
	}
}

// EvaluateEvents inspects outbox snapshots and per-event-type backlog.
func (e *Evaluator) EvaluateEvents(
	ctx context.Context,
	snapshot *appEventing.StatusSnapshot,
	eventTypes []EventTypeStatusGroup,
	window string,
	evalAt time.Time,
) []Signal {
	return NewEventDrainEvaluator(e.metrics).Evaluate(ctx, snapshot, eventTypes, window, evalAt).Signals
}

// EvaluateCache inspects cache runtime and warmup snapshots.
func (e *Evaluator) EvaluateCache(ctx context.Context, snapshot *cachegov.StatusSnapshot, window string, evalAt time.Time) []Signal {
	if snapshot == nil {
		return nil
	}
	runtimeSnapshot := snapshot.RuntimeSnapshot
	components := map[string]ComponentCache{
		nonEmpty(runtimeSnapshot.Component, "apiserver"): {Available: true, Snapshot: &runtimeSnapshot},
	}
	evaluator := NewCacheWarmupEvaluator(e.metrics)
	projection := evaluator.Evaluate(ctx, components, nil, window, evalAt)
	if len(snapshot.Warmup.LatestRuns) > 0 {
		latest := snapshot.Warmup.LatestRuns[0]
		projection.Signals = append(projection.Signals, evaluator.WarmupSignals(ctx, observabilityWarmupLatestRun{
			Trigger:     latest.Trigger,
			ErrorCount:  latest.ErrorCount,
			TargetCount: latest.TargetCount,
		}, window, evalAt)...)
	}
	return SortSignals(projection.Signals)
}

// EvaluateResilience inspects local and remote resilience snapshots with metrics.
func (e *Evaluator) EvaluateResilience(
	ctx context.Context,
	local resilienceplane.RuntimeSnapshot,
	remote map[string]ComponentResilience,
	window string,
	evalAt time.Time,
) []Signal {
	components := map[string]ComponentResilience{
		"apiserver": {Available: true, Snapshot: &local},
	}
	for name, item := range remote {
		components[name] = item
	}
	return NewResilienceProjector(e.metrics).Evaluate(ctx, components, window, evalAt).Signals
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// SortSignals orders signals by severity then id.
func SortSignals(items []Signal) []Signal {
	sort.SliceStable(items, func(i, j int) bool {
		left := severityRank(items[i].Severity)
		right := severityRank(items[j].Severity)
		if left != right {
			return left > right
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityHealthy:
		return 1
	default:
		return 0
	}
}

// OverallSeverity derives the top severity from a signal list.
func OverallSeverity(items []Signal) Severity {
	best := SeverityHealthy
	for _, item := range items {
		if severityRank(item.Severity) > severityRank(best) {
			best = item.Severity
		}
	}
	return best
}

// DomainSummaries groups signals by domain.
func DomainSummaries(items []Signal) map[Domain]DomainSummary {
	result := map[Domain]DomainSummary{
		DomainEvents:     {Severity: SeverityHealthy},
		DomainCache:      {Severity: SeverityHealthy},
		DomainResilience: {Severity: SeverityHealthy},
	}
	for _, item := range items {
		summary := result[item.Domain]
		summary.SignalCount++
		if severityRank(item.Severity) > severityRank(summary.Severity) {
			summary.Severity = item.Severity
		}
		result[item.Domain] = summary
	}
	return result
}

// ReadEventTypes loads per-event-type backlog rows.
func ReadEventTypes(ctx context.Context, sources []EventTypeStatusSource, now time.Time) []EventTypeStatusGroup {
	groups := make([]EventTypeStatusGroup, 0, len(sources))
	for _, source := range sources {
		group := EventTypeStatusGroup{Store: source.Store}
		if source.Reader == nil {
			group.Error = "event type status reader unavailable"
			groups = append(groups, group)
			continue
		}
		buckets, err := source.Reader.OutboxStatusByEventType(ctx, now)
		if err != nil {
			group.Error = err.Error()
			groups = append(groups, group)
			continue
		}
		group.Buckets = append([]outboxport.EventTypeStatusBucket(nil), buckets...)
		groups = append(groups, group)
	}
	return groups
}

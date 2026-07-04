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
	signals := append([]Signal(nil), evaluateOneResilience(ctx, "apiserver", local, e, window, evalAt)...)
	for name, item := range remote {
		if !item.Available {
			signals = append(signals, Signal{
				ID:       "resilience.component.unavailable." + name,
				Domain:   DomainResilience,
				Severity: SeverityWarning,
				Status:   "component_unavailable",
				Title:    "Component resilience snapshot unavailable: " + name,
				Evidence: map[string]interface{}{
					"component": name,
					"reason":    item.Reason,
				},
			})
			continue
		}
		if item.Snapshot != nil {
			signals = append(signals, evaluateOneResilience(ctx, name, *item.Snapshot, e, window, evalAt)...)
		}
	}
	return SortSignals(signals)
}

func evaluateOneResilience(
	ctx context.Context,
	component string,
	snapshot resilienceplane.RuntimeSnapshot,
	e *Evaluator,
	window string,
	evalAt time.Time,
) []Signal {
	signals := make([]Signal, 0)
	if !snapshot.Summary.Ready {
		signals = append(signals, Signal{
			ID:       "resilience.runtime.not_ready." + component,
			Domain:   DomainResilience,
			Severity: SeverityWarning,
			Status:   "not_ready",
			Title:    "Resilience runtime not ready: " + component,
			Evidence: map[string]interface{}{
				"component":      component,
				"degraded_count": snapshot.Summary.DegradedCount,
			},
			DashboardKey: "resilience_runtime",
		})
	}
	for _, queue := range snapshot.Queues {
		utilization := queueUtilization(queue)
		metricEvidence := []MetricEvidence{}
		if e != nil && e.metrics != nil {
			metricEvidence = append(metricEvidence, toMetricEvidence(e.metrics.Query(ctx,
				govprom.CounterIncreaseQuery(
					"queue_full_"+component+"_"+queue.Name,
					"qs_resilience_decision_total",
					window,
					queueDecisionLabels(component, queue, resilienceplane.OutcomeQueueFull),
				),
				evalAt,
			)))
		}
		if utilization >= 0.9 {
			signals = append(signals, Signal{
				ID:       "resilience.queue.critical." + component + "." + queue.Name,
				Domain:   DomainResilience,
				Severity: SeverityCritical,
				Status:   "queue_utilization_critical",
				Title:    "Queue utilization critical: " + queue.Name,
				Evidence: map[string]interface{}{
					"component":   component,
					"queue":       queue.Name,
					"depth":       queue.Depth,
					"capacity":    queue.Capacity,
					"utilization": utilization,
				},
				MetricEvidence: metricEvidence,
				DashboardKey:   "resilience_queue",
			})
			continue
		}
		if utilization >= 0.7 {
			signals = append(signals, Signal{
				ID:       "resilience.queue.warning." + component + "." + queue.Name,
				Domain:   DomainResilience,
				Severity: SeverityWarning,
				Status:   "queue_utilization_warning",
				Title:    "Queue utilization elevated: " + queue.Name,
				Evidence: map[string]interface{}{
					"component":   component,
					"queue":       queue.Name,
					"depth":       queue.Depth,
					"capacity":    queue.Capacity,
					"utilization": utilization,
				},
				MetricEvidence: metricEvidence,
				DashboardKey:   "resilience_queue",
			})
		}
	}
	for _, bp := range snapshot.Backpressure {
		utilization := backpressureUtilization(bp)
		metricEvidence := []MetricEvidence{}
		if e != nil && e.metrics != nil {
			metricEvidence = append(metricEvidence, toMetricEvidence(e.metrics.Query(ctx,
				govprom.CounterIncreaseQuery(
					"backpressure_timeout_"+component+"_"+bp.Name,
					"qs_resilience_decision_total",
					window,
					backpressureDecisionLabels(component, bp, resilienceplane.OutcomeBackpressureTimeout),
				),
				evalAt,
			)))
		}
		severity := SeverityWarning
		if utilization >= 0.9 {
			severity = SeverityCritical
		} else if utilization < 0.8 {
			continue
		}
		signals = append(signals, Signal{
			ID:       "resilience.backpressure." + component + "." + bp.Name,
			Domain:   DomainResilience,
			Severity: severity,
			Status:   "backpressure_utilization",
			Title:    "Backpressure utilization elevated: " + bp.Name,
			Evidence: map[string]interface{}{
				"component":    component,
				"name":         bp.Name,
				"in_flight":    bp.InFlight,
				"max_inflight": bp.MaxInflight,
				"utilization":  utilization,
			},
			MetricEvidence: metricEvidence,
			DashboardKey:   "resilience_backpressure",
		})
	}
	return signals
}

func queueUtilization(queue resilienceplane.QueueSnapshot) float64 {
	if queue.Capacity <= 0 {
		return 0
	}
	return float64(queue.Depth) / float64(queue.Capacity)
}

func backpressureUtilization(bp resilienceplane.BackpressureSnapshot) float64 {
	if bp.MaxInflight <= 0 {
		return 0
	}
	return float64(bp.InFlight) / float64(bp.MaxInflight)
}

func queueDecisionLabels(component string, queue resilienceplane.QueueSnapshot, outcome resilienceplane.Outcome) map[string]string {
	return map[string]string{
		"component": nonEmpty(queue.Component, component, "unknown"),
		"kind":      resilienceplane.ProtectionQueue.String(),
		"scope":     nonEmpty(queue.Name, "default"),
		"resource":  queueResource(queue),
		"strategy":  nonEmpty(queue.Strategy, "default"),
		"outcome":   outcome.String(),
	}
}

func queueResource(queue resilienceplane.QueueSnapshot) string {
	switch queue.Name {
	case "answersheet_submit", "submit":
		return "submit_queue"
	default:
		return "default"
	}
}

func backpressureDecisionLabels(component string, bp resilienceplane.BackpressureSnapshot, outcome resilienceplane.Outcome) map[string]string {
	return map[string]string{
		"component": nonEmpty(bp.Component, component, "unknown"),
		"kind":      resilienceplane.ProtectionBackpressure.String(),
		"scope":     nonEmpty(bp.Dependency, bp.Name, "default"),
		"resource":  "downstream",
		"strategy":  nonEmpty(bp.Strategy, "default"),
		"outcome":   outcome.String(),
	}
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

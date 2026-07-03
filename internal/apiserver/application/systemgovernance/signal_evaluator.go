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
	signals := make([]Signal, 0)
	if snapshot == nil {
		return signals
	}
	for _, outbox := range snapshot.Outboxes {
		if outbox.Degraded {
			signals = append(signals, Signal{
				ID:       "events.outbox.degraded." + outbox.Name,
				Domain:   DomainEvents,
				Severity: SeverityCritical,
				Status:   "degraded",
				Title:    "Outbox status reader degraded: " + outbox.Name,
				Evidence: map[string]interface{}{
					"store": outbox.Store,
					"error": outbox.Error,
				},
				DashboardKey: "events_outbox",
			})
		}
		for _, bucket := range outbox.Buckets {
			switch bucket.Status {
			case "failed":
				signals = append(signals, Signal{
					ID:       "events.outbox.failed." + outbox.Name,
					Domain:   DomainEvents,
					Severity: SeverityCritical,
					Status:   "failed",
					Title:    "Outbox has failed events",
					Evidence: map[string]interface{}{
						"store":  outbox.Store,
						"status": bucket.Status,
						"count":  bucket.Count,
					},
					DashboardKey: "events_outbox",
				})
			case "pending":
				if bucket.Count > 0 && bucket.OldestAgeSeconds >= pendingOldestAgeWarning.Seconds() {
					severity := SeverityWarning
					if bucket.OldestAgeSeconds >= 15*time.Minute.Seconds() {
						severity = SeverityCritical
					}
					metricEvidence := []MetricEvidence{}
					if e != nil && e.metrics != nil {
						metricEvidence = append(metricEvidence, toMetricEvidence(e.metrics.Query(
							ctx, govprom.GaugeQuery("outbox_pending_oldest_age_seconds",
								`max(qs_event_outbox_oldest_age_seconds{status="pending"})`,
								window, "seconds"),
							evalAt,
						)))
					}
					signals = append(signals, Signal{
						ID:       "events.outbox.pending_stale." + outbox.Name,
						Domain:   DomainEvents,
						Severity: severity,
						Status:   "pending_stale",
						Title:    "Pending outbox backlog is aging",
						Evidence: map[string]interface{}{
							"store":                 outbox.Store,
							"count":                 bucket.Count,
							"oldest_age_seconds":    bucket.OldestAgeSeconds,
							"warning_after_seconds": pendingOldestAgeWarning.Seconds(),
						},
						MetricEvidence: metricEvidence,
						DashboardKey:   "events_outbox",
					})
				}
			}
		}
	}
	for _, group := range eventTypes {
		for _, bucket := range group.Buckets {
			if bucket.Status != "pending" || bucket.Count == 0 {
				continue
			}
			age := 0.0
			if bucket.OldestCreatedAt != nil {
				age = evalAt.Sub(*bucket.OldestCreatedAt).Seconds()
			}
			if age < pendingOldestAgeWarning.Seconds() {
				continue
			}
			signals = append(signals, Signal{
				ID:       "events.type.pending_stale." + group.Store + "." + bucket.EventType,
				Domain:   DomainEvents,
				Severity: SeverityWarning,
				Status:   "event_type_backlog",
				Title:    "Event type backlog is aging: " + bucket.EventType,
				Evidence: map[string]interface{}{
					"store":              group.Store,
					"event_type":         bucket.EventType,
					"count":              bucket.Count,
					"oldest_age_seconds": age,
				},
				DashboardKey: "events_outbox_by_type",
			})
		}
		if group.Error != "" {
			signals = append(signals, Signal{
				ID:       "events.type.reader_error." + group.Store,
				Domain:   DomainEvents,
				Severity: SeverityWarning,
				Status:   "event_type_reader_error",
				Title:    "Event type status unavailable",
				Evidence: map[string]interface{}{
					"store": group.Store,
					"error": group.Error,
				},
			})
		}
	}
	return SortSignals(signals)
}

// EvaluateCache inspects cache runtime and warmup snapshots.
func (e *Evaluator) EvaluateCache(ctx context.Context, snapshot *cachegov.StatusSnapshot, window string, evalAt time.Time) []Signal {
	signals := make([]Signal, 0)
	if snapshot == nil {
		return signals
	}
	if !snapshot.Summary.Ready {
		signals = append(signals, Signal{
			ID:       "cache.runtime.not_ready",
			Domain:   DomainCache,
			Severity: SeverityCritical,
			Status:   "not_ready",
			Title:    "Cache runtime is not ready",
			Evidence: map[string]interface{}{
				"degraded_count": snapshot.Summary.DegradedCount,
			},
			ActionIDs:    []string{"cache.repair_complete"},
			DashboardKey: "cache_runtime",
		})
	}
	for _, family := range snapshot.Families {
		if !family.Available {
			signals = append(signals, Signal{
				ID:       "cache.family.unavailable." + family.Family,
				Domain:   DomainCache,
				Severity: SeverityCritical,
				Status:   "unavailable",
				Title:    "Cache family unavailable: " + family.Family,
				Evidence: map[string]interface{}{
					"family":     family.Family,
					"component":  family.Component,
					"last_error": family.LastError,
				},
				ActionIDs:    []string{"cache.repair_complete", "cache.manual_warmup"},
				DashboardKey: "cache_runtime",
			})
			continue
		}
		if family.Degraded {
			signals = append(signals, Signal{
				ID:       "cache.family.degraded." + family.Family,
				Domain:   DomainCache,
				Severity: SeverityWarning,
				Status:   "degraded",
				Title:    "Cache family degraded: " + family.Family,
				Evidence: map[string]interface{}{
					"family":    family.Family,
					"component": family.Component,
				},
				ActionIDs:    []string{"cache.manual_warmup"},
				DashboardKey: "cache_runtime",
			})
		}
	}
	if len(snapshot.Warmup.LatestRuns) > 0 {
		latest := snapshot.Warmup.LatestRuns[0]
		if latest.ErrorCount > 0 {
			signals = append(signals, Signal{
				ID:       "cache.warmup.error",
				Domain:   DomainCache,
				Severity: SeverityWarning,
				Status:   "warmup_error",
				Title:    "Cache warmup reported errors",
				Evidence: map[string]interface{}{
					"trigger":      latest.Trigger,
					"error_count":  latest.ErrorCount,
					"target_count": latest.TargetCount,
				},
				ActionIDs:    []string{"cache.manual_warmup"},
				DashboardKey: "cache_warmup",
			})
		}
	}
	_ = window
	_ = evalAt
	return SortSignals(signals)
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

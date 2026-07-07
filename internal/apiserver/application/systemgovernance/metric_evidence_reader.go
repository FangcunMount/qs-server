package systemgovernance

import (
	"context"
	"time"

	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// MetricEvidenceReader 保留PromQL 查询 construction out of 领域投影器。
type MetricEvidenceReader struct {
	metrics MetricsReader
}

func NewMetricEvidenceReader(metrics MetricsReader) MetricEvidenceReader {
	return MetricEvidenceReader{metrics: metrics}
}

func (r MetricEvidenceReader) CounterIncrease(
	ctx context.Context,
	name string,
	metric string,
	window string,
	labels map[string]string,
	evalAt time.Time,
) (MetricEvidence, bool) {
	if r.metrics == nil {
		return MetricEvidence{}, false
	}
	return toMetricEvidence(r.metrics.Query(ctx, govprom.CounterIncreaseQuery(name, metric, window, labels), evalAt)), true
}

func (r MetricEvidenceReader) InstantGauge(
	ctx context.Context,
	name string,
	metric string,
	window string,
	unit string,
	labels map[string]string,
	evalAt time.Time,
) (MetricEvidence, bool) {
	if r.metrics == nil {
		return MetricEvidence{}, false
	}
	return toMetricEvidence(r.metrics.Query(ctx, govprom.InstantGaugeQuery(name, metric, window, unit, labels), evalAt)), true
}

func (r MetricEvidenceReader) EventOutboxPendingBacklog(ctx context.Context, store, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.InstantGauge(ctx, "outbox_pending_backlog_"+store, "qs_event_outbox_backlog", window, "count", map[string]string{
		"store":  store,
		"status": "pending",
	}, evalAt)
}

func (r MetricEvidenceReader) EventOutboxPendingOldestAge(ctx context.Context, store, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.InstantGauge(ctx, "outbox_pending_oldest_age_seconds_"+store, "qs_event_outbox_oldest_age_seconds", window, "seconds", map[string]string{
		"store":  store,
		"status": "pending",
	}, evalAt)
}

func (r MetricEvidenceReader) EventOutboxStatusScrapeFailure(ctx context.Context, store, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.CounterIncrease(ctx, "outbox_status_scrape_failure_"+store, "qs_event_outbox_status_scrape_total", window, map[string]string{
		"store":   store,
		"outcome": "failure",
	}, evalAt)
}

func (r MetricEvidenceReader) EventTypePendingBacklog(ctx context.Context, store, eventType, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.InstantGauge(ctx, "outbox_event_type_pending_backlog_"+store+"_"+eventType, "qs_event_outbox_backlog_by_type", window, "count", map[string]string{
		"store":      store,
		"event_type": eventType,
		"status":     "pending",
	}, evalAt)
}

func (r MetricEvidenceReader) EventTypePendingOldestAge(ctx context.Context, store, eventType, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.InstantGauge(ctx, "outbox_event_type_pending_oldest_age_seconds_"+store+"_"+eventType, "qs_event_outbox_oldest_age_by_type_seconds", window, "seconds", map[string]string{
		"store":      store,
		"event_type": eventType,
		"status":     "pending",
	}, evalAt)
}

func (r MetricEvidenceReader) CacheFamilyAvailable(ctx context.Context, component, family, profile, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.InstantGauge(ctx, "cache_family_available_"+metricNamePart(component)+"_"+metricNamePart(family), "qs_cache_family_available", window, "bool", map[string]string{
		"component": component,
		"family":    family,
		"profile":   profile,
	}, evalAt)
}

func (r MetricEvidenceReader) CacheFamilyDegraded(ctx context.Context, component, family, profile, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.CounterIncrease(ctx, "cache_family_degraded_"+metricNamePart(component)+"_"+metricNamePart(family), "qs_cache_family_degraded_total", window, map[string]string{
		"component": component,
		"family":    family,
		"profile":   profile,
	}, evalAt)
}

func (r MetricEvidenceReader) CacheWarmupRunsError(ctx context.Context, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.CounterIncrease(ctx, "cache_warmup_runs_error", "qs_cache_warmup_runs_total", window, map[string]string{"result": "error"}, evalAt)
}

func (r MetricEvidenceReader) CacheHotsetSize(ctx context.Context, family, kind, window string, evalAt time.Time) (MetricEvidence, bool) {
	return r.InstantGauge(ctx, "cache_hotset_size_"+metricNamePart(kind), "qs_cache_hotset_size", window, "count", map[string]string{
		"family": family,
		"kind":   kind,
	}, evalAt)
}

func (r MetricEvidenceReader) ResilienceQueueFull(
	ctx context.Context,
	component string,
	queue resilienceplane.QueueSnapshot,
	window string,
	evalAt time.Time,
) (MetricEvidence, bool) {
	return r.CounterIncrease(
		ctx,
		"queue_full_"+component+"_"+queue.Name,
		"qs_resilience_decision_total",
		window,
		queueDecisionLabels(component, queue, resilienceplane.OutcomeQueueFull),
		evalAt,
	)
}

func (r MetricEvidenceReader) ResilienceBackpressureTimeout(
	ctx context.Context,
	component string,
	backpressure resilienceplane.BackpressureSnapshot,
	window string,
	evalAt time.Time,
) (MetricEvidence, bool) {
	return r.CounterIncrease(
		ctx,
		"backpressure_timeout_"+component+"_"+backpressure.Name,
		"qs_resilience_decision_total",
		window,
		backpressureDecisionLabels(component, backpressure, resilienceplane.OutcomeBackpressureTimeout),
		evalAt,
	)
}

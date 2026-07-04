package systemgovernance

import (
	"context"
	"time"

	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// MetricEvidenceReader keeps PromQL query construction out of domain projectors.
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

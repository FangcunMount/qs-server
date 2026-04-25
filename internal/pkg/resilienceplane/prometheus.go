package resilienceplane

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var resilienceDecisionTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "qs_resilience_decision_total",
		Help: "Total resilience plane decisions.",
	},
	[]string{"component", "kind", "scope", "resource", "strategy", "outcome"},
)

var resilienceQueueDepth = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "qs_resilience_queue_depth",
		Help: "Current in-memory resilience queue depth.",
	},
	[]string{"component", "scope", "resource", "strategy"},
)

var resilienceQueueStatusTotal = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "qs_resilience_queue_status_total",
		Help: "Current in-memory resilience queue status counts.",
	},
	[]string{"component", "scope", "status"},
)

var resilienceBackpressureInflight = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "qs_resilience_backpressure_inflight",
		Help: "Current backpressure in-flight operations.",
	},
	[]string{"component", "scope", "resource", "strategy"},
)

var resilienceBackpressureWaitDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "qs_resilience_backpressure_wait_duration_seconds",
		Help:    "Backpressure slot wait duration.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"component", "scope", "resource", "strategy", "outcome"},
)

type PrometheusObserver struct{}

func NewPrometheusObserver() *PrometheusObserver {
	return &PrometheusObserver{}
}

func (PrometheusObserver) ObserveDecision(_ context.Context, decision Decision) {
	subject := normalizeSubject(decision.Subject)
	resilienceDecisionTotal.WithLabelValues(
		subject.Component,
		decision.Kind.String(),
		subject.Scope,
		subject.Resource,
		subject.Strategy,
		decision.Outcome.String(),
	).Inc()
}

func ObserveQueueDepth(subject Subject, depth int) {
	normalized := normalizeSubject(subject)
	if depth < 0 {
		depth = 0
	}
	resilienceQueueDepth.WithLabelValues(
		normalized.Component,
		normalized.Scope,
		normalized.Resource,
		normalized.Strategy,
	).Set(float64(depth))
}

func ObserveQueueStatus(subject Subject, status string, count int) {
	normalized := normalizeSubject(subject)
	if status == "" {
		status = "unknown"
	}
	if count < 0 {
		count = 0
	}
	resilienceQueueStatusTotal.WithLabelValues(
		normalized.Component,
		normalized.Scope,
		status,
	).Set(float64(count))
}

func ObserveBackpressureInFlight(subject Subject, inFlight int) {
	normalized := normalizeSubject(subject)
	if inFlight < 0 {
		inFlight = 0
	}
	resilienceBackpressureInflight.WithLabelValues(
		normalized.Component,
		normalized.Scope,
		normalized.Resource,
		normalized.Strategy,
	).Set(float64(inFlight))
}

func ObserveBackpressureWaitDuration(subject Subject, outcome Outcome, duration time.Duration) {
	normalized := normalizeSubject(subject)
	if outcome == "" {
		outcome = OutcomeBackpressureAcquired
	}
	if duration < 0 {
		duration = 0
	}
	resilienceBackpressureWaitDuration.WithLabelValues(
		normalized.Component,
		normalized.Scope,
		normalized.Resource,
		normalized.Strategy,
		outcome.String(),
	).Observe(duration.Seconds())
}

package resilience

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

var collectionHTTPGateWaitSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "collection_http_gate_wait_seconds",
	Help:    "Time spent waiting for collection-server HTTP concurrency slots.",
	Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
})

var collectionGRPCInflightWaitSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "collection_grpc_inflight_wait_seconds",
	Help:    "Time spent waiting for collection-server gRPC client inflight slots.",
	Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
})

var collectionSubmitGateRejectTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "qs_collection_submit_gate_reject_total",
	Help: "Total AnswerSheet submissions rejected after the bounded gate wait.",
})

var collectionAnswerSheetSubmitTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_collection_answersheet_submit_total",
	Help: "Durable AnswerSheet acceptance attempts by outcome.",
}, []string{"outcome"})

var collectionAnswerSheetSubmitStageDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "qs_collection_answersheet_submit_stage_duration_seconds",
	Help:    "Duration of bounded reliable AnswerSheet acceptance stages.",
	Buckets: prometheus.ExponentialBuckets(0.001, 2, 12),
}, []string{"stage", "outcome"})

var collectionAnswerSheetSubmitCoalescerTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_collection_answersheet_submit_coalescer_total",
	Help: "Cross-instance AnswerSheet submit coalescing decisions by bounded outcome.",
}, []string{"outcome"})

var collectionAnswerSheetSubmitCoalescerWaitDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "qs_collection_answersheet_submit_coalescer_wait_seconds",
	Help:    "Time contenders spend waiting before durable result readback.",
	Buckets: prometheus.ExponentialBuckets(0.001, 2, 12),
}, []string{"outcome"})

var collectionAnswerSheetSubmitCoalescerRedisDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "qs_collection_answersheet_submit_coalescer_redis_seconds",
	Help:    "Redis lease-decision and completion-signal latency for AnswerSheet submit coalescing.",
	Buckets: prometheus.ExponentialBuckets(0.0005, 2, 12),
}, []string{"operation", "outcome"})

var collectionAssessmentReadinessTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_collection_assessment_readiness_total",
	Help: "Assessment readiness checks by result.",
}, []string{"status"})

var collectionSubmitToAssessmentReadyDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "qs_collection_submit_to_assessment_ready_seconds",
	Help:    "Observed time from AnswerSheet creation to Assessment readiness.",
	Buckets: prometheus.ExponentialBuckets(1, 2, 10),
})

// Offline condition (ops / EV-R015): when
// qs_worker_evaluation_payload_gate_total{class="legacy_incomplete"} stays flat
// (rate≈0) for a sustained window (e.g. 14d), retire incomplete-payload
// compatibility and treat missing model identity as invalid for new publishers.
var workerEvaluationPayloadGateTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_worker_evaluation_payload_gate_total",
	Help: "evaluation.requested payload gate classifications before Execute (EV-R015).",
}, []string{"class"})

var resilienceControlOperationTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "resilience_control_operation_total",
		Help: "Total resilience control-plane operations.",
	},
	[]string{"component", "operation", "outcome"},
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

func ObserveHTTPGateWait(duration time.Duration) {
	if duration < 0 {
		return
	}
	collectionHTTPGateWaitSeconds.Observe(duration.Seconds())
}

func ObserveGRPCInflightWait(duration time.Duration) {
	if duration < 0 {
		return
	}
	collectionGRPCInflightWaitSeconds.Observe(duration.Seconds())
}

func ObserveSubmitGateReject() {
	collectionSubmitGateRejectTotal.Inc()
}

func ObserveAnswerSheetSubmitStage(stage, outcome string, duration time.Duration) {
	if stage == "" {
		stage = "unknown"
	}
	if outcome == "" {
		outcome = "unknown"
	}
	if duration < 0 {
		return
	}
	collectionAnswerSheetSubmitStageDuration.WithLabelValues(stage, outcome).Observe(duration.Seconds())
}

func ObserveAnswerSheetSubmitOutcome(outcome string) {
	if outcome == "" {
		outcome = "unknown"
	}
	collectionAnswerSheetSubmitTotal.WithLabelValues(outcome).Inc()
}

func ObserveAnswerSheetSubmitCoalescer(outcome string) {
	if outcome == "" {
		outcome = "unknown"
	}
	collectionAnswerSheetSubmitCoalescerTotal.WithLabelValues(outcome).Inc()
}

func ObserveAnswerSheetSubmitCoalescerWait(outcome string, duration time.Duration) {
	if outcome == "" {
		outcome = "unknown"
	}
	if duration < 0 {
		return
	}
	collectionAnswerSheetSubmitCoalescerWaitDuration.WithLabelValues(outcome).Observe(duration.Seconds())
}

func ObserveAnswerSheetSubmitCoalescerRedis(operation, outcome string, duration time.Duration) {
	if operation == "" {
		operation = "unknown"
	}
	if outcome == "" {
		outcome = "unknown"
	}
	if duration < 0 {
		return
	}
	collectionAnswerSheetSubmitCoalescerRedisDuration.WithLabelValues(operation, outcome).Observe(duration.Seconds())
}

func ObserveAssessmentReadiness(status string) {
	if status == "" {
		status = "unknown"
	}
	collectionAssessmentReadinessTotal.WithLabelValues(status).Inc()
}

func ObserveEvaluationPayloadGate(class string) {
	if class == "" {
		class = "invalid"
	}
	workerEvaluationPayloadGateTotal.WithLabelValues(class).Inc()
}

func ObserveSubmitToAssessmentReady(duration time.Duration) {
	if duration >= 0 {
		collectionSubmitToAssessmentReadyDuration.Observe(duration.Seconds())
	}
}

func ObserveControlOperation(component, operation, outcome string) {
	if component == "" {
		component = "unknown"
	}
	if operation == "" {
		operation = "unknown"
	}
	if outcome == "" {
		outcome = "unknown"
	}
	resilienceControlOperationTotal.WithLabelValues(component, operation, outcome).Inc()
}

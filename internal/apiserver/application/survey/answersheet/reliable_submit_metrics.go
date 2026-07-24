package answersheet

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var durableSubmitTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_apiserver_answersheet_durable_submit_total",
	Help: "Durable AnswerSheet submissions by transactional or idempotency outcome.",
}, []string{"outcome"})

var durableSubmitStageDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "qs_apiserver_answersheet_durable_stage_duration_seconds",
	Help:    "Duration of AnswerSheet durable persistence stages.",
	Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
}, []string{"stage", "outcome"})

var durableSubmitOperationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_apiserver_answersheet_durable_operation_total",
	Help: "Durable AnswerSheet store operations by bounded operation and outcome.",
}, []string{"operation", "outcome"})

func observeDurableSubmit(outcome string) {
	if outcome == "" {
		outcome = "unknown"
	}
	durableSubmitTotal.WithLabelValues(outcome).Inc()
}

func observeDurableStage(stage, outcome string, started time.Time) {
	if outcome == "" {
		outcome = "unknown"
	}
	durableSubmitStageDuration.WithLabelValues(stage, outcome).Observe(time.Since(started).Seconds())
}

func observeDurableOperation(operation, outcome string) {
	if operation == "" {
		operation = "unknown"
	}
	if outcome == "" {
		outcome = "unknown"
	}
	durableSubmitOperationTotal.WithLabelValues(operation, outcome).Inc()
}

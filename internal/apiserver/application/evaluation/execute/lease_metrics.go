package execute

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// EV-R010: production evidence (2026-07-21) showed max run duration ~3.4s against a
// 120s Lease, so heartbeat/renew is deferred. These metrics keep family-level
// duration and lease-budget pressure observable so the decision can be revisited
// without guessing.
//
// Ops: when qs_evaluation_run_lease_budget_breach_total{threshold=~"60s|100s|lease"}
// is non-zero for a sustained window, re-open heartbeat / per-family Lease sizing.
var (
	runDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs",
		Subsystem: "evaluation",
		Name:      "run_duration_seconds",
		Help:      "Evaluation Engine Evaluate wall time by algorithm family and result (EV-R010).",
		// Cover sub-second scale tasks through the 2-minute Lease ceiling.
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 100, 120},
	}, []string{"algorithm_family", "result"})

	runLeaseBudgetBreachTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs",
		Subsystem: "evaluation",
		Name:      "run_lease_budget_breach_total",
		Help:      "Evaluation runs whose wall time crossed a Lease budget threshold (EV-R010).",
	}, []string{"algorithm_family", "threshold"})
)

const (
	leaseBudgetThreshold60s   = "60s"
	leaseBudgetThreshold100s  = "100s"
	leaseBudgetThresholdLease = "lease"
)

func observeEvaluationRunDuration(algorithmFamily, result string, duration, lease time.Duration) {
	if algorithmFamily == "" {
		algorithmFamily = "unknown"
	}
	if result == "" {
		result = "unknown"
	}
	if lease <= 0 {
		lease = defaultEvaluationRunLease
	}
	runDurationSeconds.WithLabelValues(algorithmFamily, result).Observe(duration.Seconds())
	if duration >= 60*time.Second {
		runLeaseBudgetBreachTotal.WithLabelValues(algorithmFamily, leaseBudgetThreshold60s).Inc()
	}
	if duration >= 100*time.Second {
		runLeaseBudgetBreachTotal.WithLabelValues(algorithmFamily, leaseBudgetThreshold100s).Inc()
	}
	if duration >= lease {
		runLeaseBudgetBreachTotal.WithLabelValues(algorithmFamily, leaseBudgetThresholdLease).Inc()
	}
}

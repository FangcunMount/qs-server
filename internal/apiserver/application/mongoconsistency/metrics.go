package mongoconsistency

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	auditDrift = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "drift",
		Help: "Findings from the last completed read-only Mongo consistency audit cycle.",
	}, []string{"severity", "kind"})
	auditLastSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "last_success_timestamp_seconds",
		Help: "Unix timestamp of the last completed Mongo consistency audit cycle.",
	})
	auditReady = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "ready",
		Help: "Whether the Mongo consistency audit dependencies are ready.",
	})
	auditEnabled = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "enabled",
		Help: "Whether the bounded Mongo consistency audit scheduler is enabled by configuration.",
	})
	auditErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "errors_total",
		Help: "Mongo consistency audit failures by fixed stage.",
	}, []string{"stage"})
	auditCheckpointCAS = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "checkpoint_cas_conflicts_total",
		Help: "Mongo consistency audit checkpoint compare-and-swap conflicts.",
	})
	auditBatches = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "batches_total",
		Help: "Mongo consistency audit batches by fixed phase and outcome.",
	}, []string{"phase", "outcome"})
	auditBatchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "batch_duration_seconds",
		Help: "Mongo consistency audit batch duration by fixed phase and outcome.", Buckets: prometheus.DefBuckets,
	}, []string{"phase", "outcome"})
	auditScanned = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "mongo_consistency_audit", Name: "scanned_total",
		Help: "Documents scanned by the Mongo consistency audit by fixed phase.",
	}, []string{"phase"})
)

func init() {
	prometheus.MustRegister(auditDrift, auditLastSuccess, auditReady, auditEnabled, auditErrors, auditCheckpointCAS, auditBatches, auditBatchDuration, auditScanned)
}

// SetEnabled exposes the configured scheduler state so alert rules do not
// treat the intentionally disabled stage-one rollout as a readiness failure.
func SetEnabled(enabled bool) {
	if enabled {
		auditEnabled.Set(1)
		return
	}
	auditEnabled.Set(0)
}

func observeReady(ready bool) {
	if ready {
		auditReady.Set(1)
		return
	}
	auditReady.Set(0)
}

func observeError(stage string) { auditErrors.WithLabelValues(stage).Inc() }
func observeCheckpointCAS()     { auditCheckpointCAS.Inc() }

func observeBatch(phase Phase, outcome string, duration time.Duration, scanned int) {
	auditBatches.WithLabelValues(string(phase), outcome).Inc()
	auditBatchDuration.WithLabelValues(string(phase), outcome).Observe(duration.Seconds())
	if scanned > 0 {
		auditScanned.WithLabelValues(string(phase)).Add(float64(scanned))
	}
}

func observeCompleted(completed *CompletedCycle) {
	if completed == nil {
		return
	}
	for kind, severity := range DriftSeverities {
		auditDrift.WithLabelValues(string(severity), kind).Set(float64(completed.Statistics.Findings[kind]))
	}
	auditLastSuccess.Set(float64(completed.CompletedAt.Unix()))
}

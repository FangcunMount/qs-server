package catalogreconcile

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	catalogAuditBatchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_batch_total",
		Help: "Bounded report catalog audit batches by phase and status.",
	}, []string{"phase", "status"})
	catalogAuditScannedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_scanned_total",
		Help: "Report catalog audit candidates scanned by phase.",
	}, []string{"phase"})
	catalogAuditBatchDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_batch_duration_seconds",
		Help:    "Report catalog audit batch duration by phase.",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
	}, []string{"phase", "status"})
	catalogAuditCursor = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_cursor",
		Help: "Current report catalog audit cursor by phase.",
	}, []string{"phase"})
	catalogAuditPhase = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_phase",
		Help: "Current report catalog audit phase as a one-hot gauge.",
	}, []string{"phase"})
	catalogAuditLastCompleted = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_last_completed_timestamp_seconds",
		Help: "Completion timestamp of the last complete report catalog audit cycle.",
	})
	catalogAuditDrift = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_drift",
		Help: "Drift counts from the last complete report catalog audit cycle.",
	}, []string{"kind"})
	catalogAuditErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_error_total",
		Help: "Report catalog audit errors, including timeouts and checkpoint conflicts.",
	}, []string{"kind"})
	catalogAuditReady = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "interpretation", Name: "report_catalog_audit_ready",
		Help: "Whether all required Mongo indexes for report catalog audit are present.",
	})
)

func observeAuditBatch(phase string, scanned int, _ int64, completed bool) {
	status := "advanced"
	if completed {
		status = "completed"
	}
	catalogAuditBatchTotal.WithLabelValues(phase, status).Inc()
	catalogAuditScannedTotal.WithLabelValues(phase).Add(float64(scanned))
}

func observeAuditExecution(phase, status string, seconds float64) {
	catalogAuditBatchDuration.WithLabelValues(phase, status).Observe(seconds)
}

func observeAuditCheckpoint(checkpoint AuditCheckpoint) {
	for _, phase := range []string{AuditPhaseMissing, AuditPhaseCatalog, AuditPhaseCompleted} {
		catalogAuditPhase.WithLabelValues(phase).Set(0)
		catalogAuditCursor.WithLabelValues(phase).Set(0)
	}
	catalogAuditPhase.WithLabelValues(checkpoint.Phase).Set(1)
	catalogAuditCursor.WithLabelValues(checkpoint.Phase).Set(float64(checkpoint.AfterAssessmentID))
	if checkpoint.LastCompleted == nil {
		return
	}
	catalogAuditLastCompleted.Set(float64(checkpoint.LastCompleted.CompletedAt.Unix()))
	catalogAuditDrift.WithLabelValues(DriftMissing).Set(float64(checkpoint.LastCompleted.Counts.Missing))
	catalogAuditDrift.WithLabelValues(DriftDangling).Set(float64(checkpoint.LastCompleted.Counts.Dangling))
	catalogAuditDrift.WithLabelValues(DriftAssociationMismatch).Set(float64(checkpoint.LastCompleted.Counts.AssociationMismatch))
	catalogAuditDrift.WithLabelValues(DriftWrongWinner).Set(float64(checkpoint.LastCompleted.Counts.WrongWinner))
}

func observeAuditError(kind string) {
	catalogAuditErrors.WithLabelValues(kind).Inc()
}

func observeAuditReady(ready bool) {
	if ready {
		catalogAuditReady.Set(1)
		return
	}
	catalogAuditReady.Set(0)
}

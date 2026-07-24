package attentionprojection

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var attentionFactReconcileTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "attention_fact_reconcile_total",
	Help: "Attention artifact fact reconciliation outcomes.",
}, []string{"result", "dry_run"})

var attentionFactReconcileRounds = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "attention_fact_reconcile_rounds_total",
	Help: "Attention artifact fact reconciliation rounds by result.",
}, []string{"result", "dry_run"})

var attentionFactReconcileDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "attention_fact_reconcile_duration_seconds",
	Help:    "Duration of Attention artifact fact reconciliation rounds.",
	Buckets: prometheus.DefBuckets,
}, []string{"dry_run"})

var attentionFactReconcileMissing = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "attention_fact_reconcile_missing",
	Help: "Missing Attention projection candidates observed in the latest successful round.",
}, []string{"dry_run"})

var attentionFactReconcileConsecutiveFailures = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "attention_fact_reconcile_consecutive_failures",
	Help: "Consecutive failed Attention artifact fact reconciliation rounds.",
})

func observeFactReconcile(result FactReconcileResult, dryRun bool) {
	label := strconv.FormatBool(dryRun)
	attentionFactReconcileTotal.WithLabelValues("scanned", label).Add(float64(result.Scanned))
	attentionFactReconcileTotal.WithLabelValues("missing", label).Add(float64(result.Missing))
	attentionFactReconcileTotal.WithLabelValues("existing", label).Add(float64(result.Existing))
	attentionFactReconcileTotal.WithLabelValues("mismatched", label).Add(float64(result.Mismatched))
	attentionFactReconcileTotal.WithLabelValues("created", label).Add(float64(result.Created))
	attentionFactReconcileMissing.WithLabelValues(label).Set(float64(result.Missing))
}

package catalogreconcile

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var catalogReconcileDriftTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "interpretation",
	Name:      "report_catalog_reconcile_drift_total",
	Help:      "Report catalog drift items detected by read-only reconcile (IR-R015).",
}, []string{"kind"})

func observeDrift(kind DriftKind, count int64) {
	if count <= 0 {
		return
	}
	catalogReconcileDriftTotal.WithLabelValues(kind).Add(float64(count))
}

package statistics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var statsOverviewStaleServedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "apiserver_stats_overview_stale_served_total",
	Help: "Total statistics overview responses served from in-process stale cache.",
})

func incStatsOverviewStaleServed() {
	statsOverviewStaleServedTotal.Inc()
}

var behaviorScanItemsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "apiserver_statistics_behavior_scan_items_total",
	Help: "Behavior scan items grouped by source and result.",
}, []string{"source", "result"})

var behaviorScanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "apiserver_statistics_behavior_scan_duration_seconds",
	Help:    "Duration of a complete behavior journey scan invocation.",
	Buckets: prometheus.DefBuckets,
})

var behaviorPendingReconcileTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "apiserver_statistics_behavior_pending_reconcile_items_total",
	Help: "Pending behavior reconcile items grouped by result.",
}, []string{"result"})

var behaviorPendingReconcileDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "apiserver_statistics_behavior_pending_reconcile_duration_seconds",
	Help:    "Duration of pending behavior event reconciliation.",
	Buckets: prometheus.DefBuckets,
})

var behaviorProjectionRebuildTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "apiserver_statistics_behavior_projection_rebuild_total",
	Help: "Number of bounded behavior projection rebuilds by result.",
}, []string{"result"})

var behaviorProjectionRebuildDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "apiserver_statistics_behavior_projection_rebuild_duration_seconds",
	Help:    "Duration of bounded behavior projection rebuilds.",
	Buckets: prometheus.DefBuckets,
})

func observeBehaviorScanDuration(start time.Time) {
	behaviorScanDuration.Observe(time.Since(start).Seconds())
}

func observeBehaviorScanSource(result BehaviorJourneyScanSourceResult) {
	behaviorScanItemsTotal.WithLabelValues(result.SourceName, "scanned").Add(float64(result.Scanned))
	behaviorScanItemsTotal.WithLabelValues(result.SourceName, "projected").Add(float64(result.Projected))
	behaviorScanItemsTotal.WithLabelValues(result.SourceName, "skipped").Add(float64(result.Skipped))
	behaviorScanItemsTotal.WithLabelValues(result.SourceName, "failed").Add(float64(result.Failed))
}

func observeBehaviorProjectionRebuild(start time.Time, err error) {
	behaviorProjectionRebuildDuration.Observe(time.Since(start).Seconds())
	result := "success"
	if err != nil {
		result = "failed"
	}
	behaviorProjectionRebuildTotal.WithLabelValues(result).Inc()
}

func observePendingReconcile(start time.Time, processed int, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	behaviorPendingReconcileTotal.WithLabelValues(result).Add(float64(processed))
	behaviorPendingReconcileDuration.Observe(time.Since(start).Seconds())
}

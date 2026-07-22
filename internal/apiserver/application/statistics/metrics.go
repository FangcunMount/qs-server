package statistics

import (
	"strconv"
	"time"

	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var statisticsRunTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "run_total",
	Help: "Statistics runs grouped by mode, trigger, and terminal status.",
}, []string{"mode", "trigger", "status"})

var statisticsRunDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "run_duration_seconds",
	Help: "Statistics run duration grouped by mode and trigger.", Buckets: prometheus.DefBuckets,
}, []string{"mode", "trigger"})

var statisticsStageFailureTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "stage_failure_total",
	Help: "Statistics failures grouped by stage and structured error code.",
}, []string{"stage", "code"})

var statisticsProcessedRows = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "processed_rows_total",
	Help: "Rows processed by Statistics collectors and projections.",
}, []string{"phase", "name", "result"})

var statisticsCachePublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "cache_publish_total",
	Help: "Statistics cache generation publication attempts.",
}, []string{"operation", "result"})

var statisticsFreshnessLagDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "freshness_lag_days",
	Help: "Statistics published business-day lag by configured organization.",
}, []string{"org_id"})

var statisticsLastPublishSuccess = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "last_publish_success_unixtime",
	Help: "Unix timestamp of the last successful Statistics publish run by organization.",
}, []string{"org_id"})

var statisticsPublishedAsOf = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "published_as_of_unixtime",
	Help: "Unix timestamp of the Shanghai business day published by the last successful run.",
}, []string{"org_id"})

var statisticsStaleResponseTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics", Name: "stale_response_total",
	Help: "Statistics responses served with stale freshness metadata.",
})

func observeStatisticsRun(start time.Time, mode statisticsDomain.RunMode, trigger string, run *Run, err error) {
	status := "rejected"
	if run != nil {
		status = string(run.Status)
	} else if err == nil {
		status = "succeeded"
	}
	statisticsRunTotal.WithLabelValues(string(mode), trigger, status).Inc()
	statisticsRunDuration.WithLabelValues(string(mode), trigger).Observe(time.Since(start).Seconds())
	if run != nil && mode == statisticsDomain.RunModePublish && run.Status == statisticsDomain.RunStatusSucceeded {
		orgID := strconv.FormatInt(run.OrgID, 10)
		statisticsLastPublishSuccess.WithLabelValues(orgID).Set(float64(time.Now().Unix()))
		statisticsPublishedAsOf.WithLabelValues(orgID).Set(float64(run.AsOfDate.Unix()))
	}
}

func observeStatisticsStageFailure(stage, code string) {
	statisticsStageFailureTotal.WithLabelValues(stage, code).Inc()
}

func observeCollectorResult(item statisticsDomain.CollectResult) {
	statisticsProcessedRows.WithLabelValues("collector", item.Collector, "source").Add(float64(item.SourceCount))
	statisticsProcessedRows.WithLabelValues("collector", item.Collector, "inserted").Add(float64(item.InsertedCount))
	statisticsProcessedRows.WithLabelValues("collector", item.Collector, "existing").Add(float64(item.ExistingCount))
	statisticsProcessedRows.WithLabelValues("collector", item.Collector, "conflict").Add(float64(item.ConflictCount))
}

func observeProjectionResult(item statisticsDomain.ProjectionResult) {
	statisticsProcessedRows.WithLabelValues("projection", item.Name, "rows").Add(float64(item.Rows))
}

func observeCachePublish(operation string, err error) {
	result := "succeeded"
	if err != nil {
		result = "failed"
	}
	statisticsCachePublishTotal.WithLabelValues(operation, result).Inc()
}

func observeFreshness(orgID int64, asOf, previousCompleteDay time.Time) {
	lag := previousCompleteDay.Sub(asOf).Hours() / 24
	if lag < 0 {
		lag = 0
	}
	statisticsFreshnessLagDays.WithLabelValues(strconv.FormatInt(orgID, 10)).Set(lag)
	if asOf.Before(previousCompleteDay) {
		statisticsStaleResponseTotal.Inc()
	}
}

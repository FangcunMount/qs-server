package statisticsv2

import (
	"strconv"
	"time"

	domainv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var statisticsV2RunTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "run_total",
	Help: "Statistics V2 runs grouped by mode, trigger, and terminal status.",
}, []string{"mode", "trigger", "status"})

var statisticsV2RunDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "run_duration_seconds",
	Help: "Statistics V2 run duration grouped by mode and trigger.", Buckets: prometheus.DefBuckets,
}, []string{"mode", "trigger"})

var statisticsV2StageFailureTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "stage_failure_total",
	Help: "Statistics V2 failures grouped by stage and structured error code.",
}, []string{"stage", "code"})

var statisticsV2ProcessedRows = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "processed_rows_total",
	Help: "Rows processed by Statistics V2 collectors and projections.",
}, []string{"phase", "name", "result"})

var statisticsV2CachePublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "cache_publish_total",
	Help: "Statistics V2 cache generation publication attempts.",
}, []string{"operation", "result"})

var statisticsV2FreshnessLagDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "freshness_lag_days",
	Help: "Statistics V2 published business-day lag by configured organization.",
}, []string{"org_id"})

var statisticsV2LastPublishSuccess = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "last_publish_success_unixtime",
	Help: "Unix timestamp of the last successful Statistics V2 publish run by organization.",
}, []string{"org_id"})

var statisticsV2PublishedAsOf = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "published_as_of_unixtime",
	Help: "Unix timestamp of the Shanghai business day published by the last successful run.",
}, []string{"org_id"})

var statisticsV2StaleResponseTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics_v2", Name: "stale_response_total",
	Help: "Statistics V2 responses served with stale freshness metadata.",
})

func observeStatisticsRun(start time.Time, mode domainv2.RunMode, trigger string, run *Run, err error) {
	status := "rejected"
	if run != nil {
		status = string(run.Status)
	} else if err == nil {
		status = "succeeded"
	}
	statisticsV2RunTotal.WithLabelValues(string(mode), trigger, status).Inc()
	statisticsV2RunDuration.WithLabelValues(string(mode), trigger).Observe(time.Since(start).Seconds())
	if run != nil && mode == domainv2.RunModePublish && run.Status == domainv2.RunStatusSucceeded {
		orgID := strconv.FormatInt(run.OrgID, 10)
		statisticsV2LastPublishSuccess.WithLabelValues(orgID).Set(float64(time.Now().Unix()))
		statisticsV2PublishedAsOf.WithLabelValues(orgID).Set(float64(run.AsOfDate.Unix()))
	}
}

func observeStatisticsStageFailure(stage, code string) {
	statisticsV2StageFailureTotal.WithLabelValues(stage, code).Inc()
}

func observeCollectorResult(item domainv2.CollectResult) {
	statisticsV2ProcessedRows.WithLabelValues("collector", item.Collector, "source").Add(float64(item.SourceCount))
	statisticsV2ProcessedRows.WithLabelValues("collector", item.Collector, "inserted").Add(float64(item.InsertedCount))
	statisticsV2ProcessedRows.WithLabelValues("collector", item.Collector, "existing").Add(float64(item.ExistingCount))
	statisticsV2ProcessedRows.WithLabelValues("collector", item.Collector, "conflict").Add(float64(item.ConflictCount))
}

func observeProjectionResult(item domainv2.ProjectionResult) {
	statisticsV2ProcessedRows.WithLabelValues("projection", item.Name, "rows").Add(float64(item.Rows))
}

func observeCachePublish(operation string, err error) {
	result := "succeeded"
	if err != nil {
		result = "failed"
	}
	statisticsV2CachePublishTotal.WithLabelValues(operation, result).Inc()
}

func observeFreshness(orgID int64, asOf, previousCompleteDay time.Time) {
	lag := previousCompleteDay.Sub(asOf).Hours() / 24
	if lag < 0 {
		lag = 0
	}
	statisticsV2FreshnessLagDays.WithLabelValues(strconv.FormatInt(orgID, 10)).Set(lag)
	if asOf.Before(previousCompleteDay) {
		statisticsV2StaleResponseTotal.Inc()
	}
}

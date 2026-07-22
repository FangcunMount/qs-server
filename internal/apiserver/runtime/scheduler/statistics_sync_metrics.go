package scheduler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var statisticsSchedulerOrgTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "statistics_scheduler", Name: "organization_total",
	Help: "Nightly statistics organization executions grouped by result.",
}, []string{"result"})

func observeStatisticsSchedulerOrg(result string) {
	statisticsSchedulerOrgTotal.WithLabelValues(result).Inc()
}

package statistics

import (
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

var statsQuestionnaireStaleServedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "apiserver_stats_questionnaire_stale_served_total",
	Help: "Total questionnaire statistics responses served from in-process stale cache.",
})

func incStatsQuestionnaireStaleServed() {
	statsQuestionnaireStaleServedTotal.Inc()
}

package observe

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	signalNotifyTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_signal_notify_total",
			Help: "Total cache signal notify attempts.",
		},
		[]string{"signal", "service"},
	)
	signalNotifyFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_signal_notify_failed_total",
			Help: "Total cache signal notify failures.",
		},
		[]string{"signal", "service"},
	)
)

func IncSignalNotify(signalName, service string) {
	if service == "" {
		service = "unknown"
	}
	signalNotifyTotal.WithLabelValues(signalName, service).Inc()
}

func IncSignalNotifyFailed(signalName, service string) {
	if service == "" {
		service = "unknown"
	}
	signalNotifyFailedTotal.WithLabelValues(signalName, service).Inc()
}

package reportstatus

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	signalingNotifyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "signaling_notify_total",
		Help: "Total report status signaling notify attempts.",
	}, []string{"signal_name", "service"})
	signalingNotifyFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "signaling_notify_failed_total",
		Help: "Total failed report status signaling notify attempts.",
	}, []string{"signal_name", "service"})
	signalingWatchReceivedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "signaling_watch_received_total",
		Help: "Total report status signals received by watcher.",
	}, []string{"signal_name", "service"})
	signalingWatchDecodeFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "signaling_watch_decode_failed_total",
		Help: "Total report status signal decode failures in watcher.",
	}, []string{"signal_name", "service"})
	signalingWatchReconnectTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "signaling_watch_reconnect_total",
		Help: "Total report status signal watcher reconnect attempts.",
	}, []string{"signal_name", "service"})

	reportWaitActiveWaiters = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "report_wait_active_waiters",
		Help: "Current active wait-report waiters.",
	})
	reportWaitRegisteredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "report_wait_registered_total",
		Help: "Total wait-report waiter registrations.",
	})
	reportWaitUnregisteredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "report_wait_unregistered_total",
		Help: "Total wait-report waiter unregistrations.",
	})
	reportWaitWakeupTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "report_wait_wakeup_total",
		Help: "Total wait-report wakeups from signaling.",
	})
	reportWaitTimeoutTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "report_wait_timeout_total",
		Help: "Total wait-report long-poll timeouts.",
	})

	waitReportRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wait_report_requests_total",
		Help: "Total wait-report requests.",
	})
	waitReportCompletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wait_report_completed_total",
		Help: "Total wait-report responses with completed status.",
	})
	waitReportProcessingTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wait_report_processing_total",
		Help: "Total wait-report responses still processing.",
	})
	waitReportFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wait_report_failed_total",
		Help: "Total wait-report responses with failed status.",
	})
	waitReportRedisHitTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wait_report_redis_hit_total",
		Help: "Total wait-report Redis status cache hits.",
	})
	waitReportRedisMissTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wait_report_redis_miss_total",
		Help: "Total wait-report Redis status cache misses.",
	})
	waitReportDBFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wait_report_db_fallback_total",
		Help: "Total wait-report DB fallback lookups.",
	})
	waitReportSignalWakeupTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wait_report_signal_wakeup_total",
		Help: "Total wait-report wakeups triggered by signaling.",
	})

	reportStatusGetTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "report_status_get_total",
		Help: "Total report status cache get operations.",
	}, []string{"result", "status"})
	reportStatusGetFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "report_status_get_failed_total",
		Help: "Total failed report status cache get operations.",
	}, []string{"status"})
	reportStatusSetTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "report_status_set_total",
		Help: "Total report status cache set operations.",
	}, []string{"status"})
	reportStatusSetFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "report_status_set_failed_total",
		Help: "Total failed report status cache set operations.",
	}, []string{"status"})
)

func IncNotify(signalName, service string) {
	signalingNotifyTotal.WithLabelValues(signalName, service).Inc()
}

func IncNotifyFailed(signalName, service string) {
	signalingNotifyFailedTotal.WithLabelValues(signalName, service).Inc()
}

func IncWatchReceived(signalName, service string) {
	signalingWatchReceivedTotal.WithLabelValues(signalName, service).Inc()
}

func IncWatchDecodeFailed(signalName, service string) {
	signalingWatchDecodeFailedTotal.WithLabelValues(signalName, service).Inc()
}

func IncWatchReconnect(signalName, service string) {
	signalingWatchReconnectTotal.WithLabelValues(signalName, service).Inc()
}

func IncStatusGet(result, status string) {
	reportStatusGetTotal.WithLabelValues(result, status).Inc()
}

func IncStatusGetFailed(status string) {
	reportStatusGetFailedTotal.WithLabelValues(status).Inc()
}

func IncStatusSet(status string) {
	reportStatusSetTotal.WithLabelValues(status).Inc()
}

func IncStatusSetFailed(status string) {
	reportStatusSetFailedTotal.WithLabelValues(status).Inc()
}

func SetActiveWaiters(count int) {
	reportWaitActiveWaiters.Set(float64(count))
}

func IncWaitRegistered() {
	reportWaitRegisteredTotal.Inc()
}

func IncWaitUnregistered() {
	reportWaitUnregisteredTotal.Inc()
}

func IncWaitWakeup() {
	reportWaitWakeupTotal.Inc()
}

func IncWaitTimeout() {
	reportWaitTimeoutTotal.Inc()
}

func IncWaitReportRequest() {
	waitReportRequestsTotal.Inc()
}

func IncWaitReportCompleted() {
	waitReportCompletedTotal.Inc()
}

func IncWaitReportProcessing() {
	waitReportProcessingTotal.Inc()
}

func IncWaitReportFailed() {
	waitReportFailedTotal.Inc()
}

func IncWaitReportRedisHit() {
	waitReportRedisHitTotal.Inc()
}

func IncWaitReportRedisMiss() {
	waitReportRedisMissTotal.Inc()
}

func IncWaitReportDBFallback() {
	waitReportDBFallbackTotal.Inc()
}

func IncWaitReportSignalWakeup() {
	waitReportSignalWakeupTotal.Inc()
}

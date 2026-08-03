package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var taskExpirationNotificationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "worker", Name: "task_expiration_notification_total",
	Help: "Task expiration notification decisions grouped by reason and result.",
}, []string{"reason", "result"})

func ObserveTaskExpirationNotification(reason, result string) {
	taskExpirationNotificationTotal.WithLabelValues(reason, result).Inc()
}

package testeeaccess

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	testeeAccessTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "report_status_testee_access_total",
		Help: "Total IAM User to Testee access checks before report status reads.",
	}, []string{"result"})
	testeeAccessDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "report_status_testee_access_duration_seconds",
		Help:    "Latency of IAM User to Testee access checks before report status reads.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
	})
)

func observeTesteeAccess(result string, duration time.Duration) {
	switch result {
	case "allowed", "denied", "error", "misconfigured":
	default:
		result = "error"
	}
	testeeAccessTotal.WithLabelValues(result).Inc()
	testeeAccessDuration.Observe(duration.Seconds())
}

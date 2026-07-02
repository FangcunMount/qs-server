package concurrency

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var httpGateWaitSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "collection_http_gate_wait_seconds",
	Help:    "Time spent waiting for collection-server HTTP concurrency slots.",
	Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
})

func observeHTTPGateWait(duration time.Duration) {
	if duration < 0 {
		return
	}
	httpGateWaitSeconds.Observe(duration.Seconds())
}

package admission

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpGateWaitSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "collection_http_gate_wait_seconds",
		Help:    "Time spent waiting for collection-server HTTP concurrency slots.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
	})
	grpcInflightWaitSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "collection_grpc_inflight_wait_seconds",
		Help:    "Time spent waiting for collection-server gRPC client inflight slots.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
	})
)

// ObserveHTTPGateWait 记录 HTTP 槽位等待时长。
func ObserveHTTPGateWait(duration time.Duration) {
	if duration < 0 {
		return
	}
	httpGateWaitSeconds.Observe(duration.Seconds())
}

// ObserveGRPCInflightWait 记录 gRPC inflight 槽位等待时长。
func ObserveGRPCInflightWait(duration time.Duration) {
	if duration < 0 {
		return
	}
	grpcInflightWaitSeconds.Observe(duration.Seconds())
}

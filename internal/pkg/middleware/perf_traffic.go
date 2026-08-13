package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	perfRunIDHeader    = "X-Perf-Run-ID"
	trafficOriginPerf  = "perf"
	trafficOriginOther = "other"
)

var perfTrafficRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_perf_traffic_requests_total",
	Help: "Business HTTP and WebSocket requests classified as k6 perf traffic or other concurrent traffic.",
}, []string{"origin"})

func init() {
	// Pre-initialize both bounded label values so a clean traffic window still
	// exports explicit zero-valued evidence for the perf orchestrator.
	perfTrafficRequestsTotal.WithLabelValues(trafficOriginPerf)
	perfTrafficRequestsTotal.WithLabelValues(trafficOriginOther)
}

// PerfTrafficEvidence classifies business traffic without attaching the run ID
// as a Prometheus label. The bounded origin label keeps cardinality stable while
// allowing before/after snapshots to prove that a declared isolated window did
// not contain unrelated requests.
func PerfTrafficEvidence() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !excludedFromPerfTrafficEvidence(c.Request.URL.Path) {
			perfTrafficRequestsTotal.WithLabelValues(classifyPerfTrafficOrigin(c.GetHeader(perfRunIDHeader))).Inc()
		}
		c.Next()
	}
}

func classifyPerfTrafficOrigin(runID string) string {
	if strings.TrimSpace(runID) != "" {
		return trafficOriginPerf
	}
	return trafficOriginOther
}

func excludedFromPerfTrafficEvidence(path string) bool {
	switch path {
	case "/health", "/healthz", "/readyz", "/serve-readyz", "/metrics", "/ping", "/version":
		return true
	default:
		return strings.HasPrefix(path, "/debug/pprof")
	}
}

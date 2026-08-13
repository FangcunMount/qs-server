package middleware

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/gin-gonic/gin"
)

const (
	perfRunIDHeader    = "X-Perf-Run-ID"
	trafficOriginPerf  = "perf"
	trafficOriginOther = "other"
)

// PerfTrafficEvidence classifies business traffic without attaching the run ID
// as a Prometheus label. The bounded origin label keeps cardinality stable while
// allowing before/after snapshots to prove that a declared isolated window did
// not contain unrelated requests.
func PerfTrafficEvidence() gin.HandlerFunc {
	return perfTrafficEvidence(resilience.ObservePerfTrafficRequest)
}

func perfTrafficEvidence(observe func(origin string)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !excludedFromPerfTrafficEvidence(c.Request.URL.Path) {
			observe(classifyPerfTrafficOrigin(c.GetHeader(perfRunIDHeader)))
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

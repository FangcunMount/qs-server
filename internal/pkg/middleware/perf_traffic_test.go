package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestClassifyPerfTrafficOrigin(t *testing.T) {
	if got := classifyPerfTrafficOrigin("run-1"); got != trafficOriginPerf {
		t.Fatalf("origin = %q, want %q", got, trafficOriginPerf)
	}
	if got := classifyPerfTrafficOrigin("  "); got != trafficOriginOther {
		t.Fatalf("origin = %q, want %q", got, trafficOriginOther)
	}
}

func TestPerfTrafficEvidenceExcludesOnlyObservabilityRoutes(t *testing.T) {
	for _, path := range []string{"/health", "/healthz", "/readyz", "/serve-readyz", "/metrics", "/debug/pprof/goroutine"} {
		if !excludedFromPerfTrafficEvidence(path) {
			t.Fatalf("path %q must be excluded", path)
		}
	}
	for _, path := range []string{"/api/v1/answersheets", "/internal/v1/system-governance/resilience", "/swagger-ui/index.html"} {
		if excludedFromPerfTrafficEvidence(path) {
			t.Fatalf("path %q must remain traffic evidence", path)
		}
	}
}

func TestPerfTrafficEvidenceCountsPerfAndOtherRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(PerfTrafficEvidence())
	engine.GET("/api/v1/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	engine.GET("/readyz", func(c *gin.Context) { c.Status(http.StatusOK) })

	perfBefore := testutil.ToFloat64(perfTrafficRequestsTotal.WithLabelValues(trafficOriginPerf))
	otherBefore := testutil.ToFloat64(perfTrafficRequestsTotal.WithLabelValues(trafficOriginOther))

	perfRequest := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	perfRequest.Header.Set(perfRunIDHeader, "run-1")
	engine.ServeHTTP(httptest.NewRecorder(), perfRequest)
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/readyz", nil))

	perfAfter := testutil.ToFloat64(perfTrafficRequestsTotal.WithLabelValues(trafficOriginPerf))
	otherAfter := testutil.ToFloat64(perfTrafficRequestsTotal.WithLabelValues(trafficOriginOther))
	if perfAfter-perfBefore != 1 || otherAfter-otherBefore != 1 {
		t.Fatalf("traffic deltas = perf %.0f other %.0f, want 1/1", perfAfter-perfBefore, otherAfter-otherBefore)
	}
}

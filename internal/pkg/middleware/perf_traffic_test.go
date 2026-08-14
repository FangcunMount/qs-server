package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
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
	var observed []string
	engine := gin.New()
	engine.Use(perfTrafficEvidence(func(origin string) {
		observed = append(observed, origin)
	}))
	engine.GET("/api/v1/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	engine.GET("/readyz", func(c *gin.Context) { c.Status(http.StatusOK) })

	perfRequest := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	perfRequest.Header.Set(perfRunIDHeader, "run-1")
	engine.ServeHTTP(httptest.NewRecorder(), perfRequest)
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if want := []string{trafficOriginPerf, trafficOriginOther}; !reflect.DeepEqual(observed, want) {
		t.Fatalf("observed origins = %v, want %v", observed, want)
	}
}

func TestPerfTrafficEvidenceIsIdempotentWithinOneHandlerChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var observed []string
	middleware := perfTrafficEvidence(func(origin string) {
		observed = append(observed, origin)
	})
	engine := gin.New()
	engine.Use(middleware, middleware)
	engine.GET("/api/v1/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	request.Header.Set(perfRunIDHeader, "run-1")
	engine.ServeHTTP(httptest.NewRecorder(), request)

	if want := []string{trafficOriginPerf}; !reflect.DeepEqual(observed, want) {
		t.Fatalf("observed origins = %v, want %v", observed, want)
	}
}

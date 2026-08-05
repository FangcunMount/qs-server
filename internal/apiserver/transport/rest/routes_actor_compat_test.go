package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestDeprecatedPractitionerRouteIsObserved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	practitioners := engine.Group("/api/v1/practitioners")
	practitioners.Use(observeDeprecatedPractitionerRoute)
	practitioners.GET("", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	before := testutil.ToFloat64(deprecatedPractitionerRouteTotal)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/practitioners", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if delta := testutil.ToFloat64(deprecatedPractitionerRouteTotal) - before; delta != 1 {
		t.Fatalf("metric delta = %v, want 1", delta)
	}
}

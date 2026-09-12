package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRetiredActorRoutesNeedNoDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// Exercise production registrar composition with no retired service dependencies.
	rate := options.NewRateLimitOptions()
	rate.Enabled = false
	router := NewRouter(Deps{RateLimit: rate})
	router.RegisterRoutes(engine)
	for _, path := range []string{"/staff", "/staff/7", "/clinicians/me", "/clinicians/me/testees", "/clinicians/me/reports", "/clinicians/me/reports/7", "/clinicians/me/assessment-entries/7/reactivate", "/clinicians/7/bind-operator", "/clinicians/7/unbind-operator", "/practitioners/me", "/practitioners/me/workbench/queues/summary", "/practitioners/7/bind-operator"} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1"+path, nil))
			if rec.Code != http.StatusGone {
				t.Fatalf("%s %s = %d, want 410", method, path, rec.Code)
			}
		}
	}
}

func TestRetiredStatisticsRoutesBypassUnavailableAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rate := options.NewRateLimitOptions()
	rate.Enabled = false
	NewRouter(Deps{RateLimit: rate}).RegisterRoutes(engine)
	for _, path := range []string{"/api/v2/statistics/clinicians/me", "/api/v2/statistics/clinicians/me/overview"} {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusGone {
			t.Fatalf("%s = %d, want 410 without identity/database dependencies", path, rec.Code)
		}
	}
}

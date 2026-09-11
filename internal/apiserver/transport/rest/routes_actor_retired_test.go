package rest

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRetiredActorRoutesNeedNoDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router := &Router{}
	router.registerActorProtectedRoutes(engine.Group("/api/v1"))
	for _, path := range []string{"/staff", "/staff/7", "/clinicians/me", "/clinicians/me/testees", "/clinicians/me/assessment-entries/7/reactivate", "/clinicians/7/bind-operator", "/clinicians/7/unbind-operator", "/practitioners/me", "/practitioners/me/workbench/queues/summary", "/practitioners/7/bind-operator"} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1"+path, nil))
			if rec.Code != http.StatusGone {
				t.Fatalf("%s %s = %d, want 410", method, path, rec.Code)
			}
		}
	}
}

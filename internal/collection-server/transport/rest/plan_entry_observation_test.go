package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestTaskEntryObservationIncludesRejectedAuthenticationWithoutReordering(t *testing.T) {
	for _, route := range []struct{ kind, template, path string }{
		{"canonical", "/plan-task-entries/:task_id", "/api/v1/plan-task-entries/42"},
		{"legacy_token", "/plan-task-entries/:task_id/:token", "/api/v1/plan-task-entries/42/private-token"},
	} {
		for _, status := range []int{200, 401, 403, 404, 503} {
			result := "rejected"
			if status == 200 {
				result = "accepted"
			}
			before := testutil.ToFloat64(planEntryRouteTotal.WithLabelValues(route.kind, result))
			engine := gin.New()
			api := engine.Group("/api/v1")
			api.Use(observePlanEntryRoutes())
			called := false
			if status == 401 {
				(&Router{}).applyIAMAuth(api, nil)
			} // real unavailable-auth middleware aborts before handler
			api.GET(route.template, func(c *gin.Context) { called = true; c.Status(status) })
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, route.path, nil))
			if status == 401 {
				if called {
					t.Fatal("observer changed authentication order")
				}
				// The unavailable auth contract is 503, still a rejected invocation.
				if recorder.Code != 503 {
					t.Fatalf("status=%d", recorder.Code)
				}
			} else if !called || recorder.Code != status {
				t.Fatalf("handler=%t status=%d want=%d", called, recorder.Code, status)
			}
			if delta := testutil.ToFloat64(planEntryRouteTotal.WithLabelValues(route.kind, result)) - before; delta != 1 {
				t.Fatalf("%s/%s delta=%g", route.kind, result, delta)
			}
		}
	}
}

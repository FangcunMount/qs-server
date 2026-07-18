package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/gin-gonic/gin"
)

func TestCatalogConcurrencyHandlersBypassGateOnL1Hit(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	gate := concurrency.NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected to acquire sole catalog slot")
	}

	opts := options.NewOptions()
	opts.Concurrency.MaxCatalogConcurrency = 1
	opts.Concurrency.CatalogMaxWaitMs = 100
	c := mustNewCollectionContainer(t, opts, nil, nil, nil)
	c.InitializeRuntimeClients(container.ClientBundle{})

	// Generic catalogue has no local projection cache; verify the admission
	// bypass policy independently from a legacy scale DTO.
	router := &Router{container: c}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessment-models/ABC", nil)
	ctx.Params = gin.Params{{Key: "code", Value: "ABC"}}
	ctx.FullPath()

	peek := func(*gin.Context) bool { return true }

	handlers := catalogConcurrencyHandlers(gate, time.Second, peek, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	handlers[0](ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (L1 bypass)", recorder.Code)
	}
	_ = router
}

func TestCatalogConcurrencyHandlersWaitWhenL1MissAndSlotAvailable(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	gate := concurrency.NewGate(2)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessment-models/missing", nil)

	handlers := catalogConcurrencyHandlers(gate, time.Second, func(*gin.Context) bool { return false }, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	handlers[0](ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestTryQueryConcurrencyHandlersRejectWhenSlotsFull(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	gate := concurrency.NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected to acquire sole query slot")
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/answersheets/1/assessment", nil)

	handlers := tryQueryConcurrencyHandlers(gate, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	handlers[0](c)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestAdmissionPolicyRoutesReportStatusThroughTryGate(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	gate := concurrency.NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected to acquire sole query slot")
	}
	policy := AdmissionPolicy{queryGate: gate}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/1/report-status", nil)

	handlers := policy.Wrap(admissionReportStatus, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	handlers[0](c)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestAdmissionPolicyRoutesWaitReportThroughDegradeGate(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	gate := concurrency.NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected to acquire sole wait-report slot")
	}
	policy := AdmissionPolicy{
		waitReportGate: gate,
		waitReport: &options.WaitReportOptions{
			DegradeImmediateEnabled:  true,
			DegradeRetryAfterSeconds: 7,
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/1/wait-report", nil)

	handlers := policy.Wrap(admissionWaitReport, func(c *gin.Context) {
		c.Status(http.StatusTeapot)
	})
	handlers[0](c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want degraded 200", recorder.Code)
	}
	if got := recorder.Header().Get("Retry-After"); got != "7" {
		t.Fatalf("Retry-After = %q, want 7", got)
	}
}

func TestAdmissionPolicyRouteMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	run := func(t *testing.T, route admissionRoute, gate *concurrency.Gate, occupy bool, want int) {
		t.Helper()
		if occupy {
			if !gate.TryAcquire() {
				t.Fatal("expected to occupy gate slot")
			}
		}
		policy := AdmissionPolicy{
			catalogGate:    gate,
			queryGate:      gate,
			submitGate:     gate,
			waitReportGate: gate,
			maxWait:        time.Second,
			catalogMaxWait: time.Second,
			waitReport: &options.WaitReportOptions{
				DegradeImmediateEnabled:  true,
				DegradeRetryAfterSeconds: 5,
			},
		}
		recorder := httptest.NewRecorder()
		c, engine := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

		handlers := policy.Wrap(route, func(c *gin.Context) {
			c.Status(http.StatusTeapot)
		})
		engine.GET("/test", handlers...)
		engine.ServeHTTP(recorder, c.Request)
		if recorder.Code != want {
			t.Fatalf("route=%d status=%d want=%d", route, recorder.Code, want)
		}
	}

	t.Run("report_status_try_reject", func(t *testing.T) {
		run(t, admissionReportStatus, concurrency.NewGate(1), true, http.StatusServiceUnavailable)
	})
	t.Run("query_wait_pass", func(t *testing.T) {
		run(t, admissionQuery, concurrency.NewGate(2), false, http.StatusTeapot)
	})
	t.Run("submit_wait_pass", func(t *testing.T) {
		run(t, admissionSubmit, concurrency.NewGate(2), false, http.StatusTeapot)
	})
	t.Run("wait_report_degrade", func(t *testing.T) {
		run(t, admissionWaitReport, concurrency.NewGate(1), true, http.StatusOK)
	})
}

func TestAdmissionPolicyCatalogUsesWaitGateOnL1Miss(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	gate := concurrency.NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected to occupy catalog slot")
	}
	policy := AdmissionPolicy{
		catalogGate:    gate,
		catalogMaxWait: time.Second,
		catalogPeek:    func(*gin.Context) bool { return false },
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessment-models/missing", nil)

	handlers := policy.Wrap(admissionCatalog, func(c *gin.Context) {
		c.Status(http.StatusTeapot)
	})
	handlers[0](c)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestRouterConcurrencyMaxWaitFromOptions(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.Concurrency.MaxWaitMs = 2500
	opts.Concurrency.CatalogMaxWaitMs = 800
	router := &Router{container: mustNewCollectionContainer(t, opts, nil, nil, nil)}
	if got := router.concurrencyMaxWait().Milliseconds(); got != 2500 {
		t.Fatalf("concurrencyMaxWait = %dms, want 2500ms", got)
	}
	if got := router.catalogMaxWait().Milliseconds(); got != 800 {
		t.Fatalf("catalogMaxWait = %dms, want 800ms", got)
	}
}

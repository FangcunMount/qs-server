package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
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
	c := container.NewContainer(opts, nil, nil, nil)
	c.InitializeRuntimeClients(container.ClientBundle{})

	cache := scale.NewLocalCatalogCache(scale.LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 8})
	cache.SetDetail("ABC", &scale.ScaleResponse{Code: "ABC"})
	scaleSvc := scale.NewQueryService(nil, cache, true)

	// inject scale service via reflection is heavy; use router peek with manual setup
	router := &Router{container: c}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/scales/ABC", nil)
	ctx.Params = gin.Params{{Key: "code", Value: "ABC"}}
	ctx.FullPath()

	peek := func(c *gin.Context) bool {
		return scaleSvc.HasCachedDetail(c.Param("code"))
	}

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
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/scales/missing", nil)

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

func TestRouterConcurrencyMaxWaitFromOptions(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.Concurrency.MaxWaitMs = 2500
	opts.Concurrency.CatalogMaxWaitMs = 800
	router := &Router{container: container.NewContainer(opts, nil, nil, nil)}
	if got := router.concurrencyMaxWait().Milliseconds(); got != 2500 {
		t.Fatalf("concurrencyMaxWait = %dms, want 2500ms", got)
	}
	if got := router.catalogMaxWait().Milliseconds(); got != 800 {
		t.Fatalf("catalogMaxWait = %dms, want 800ms", got)
	}
}

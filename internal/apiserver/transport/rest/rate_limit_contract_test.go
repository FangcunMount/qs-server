package rest

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/apiserver/resilience/subsystem"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

func TestRateLimitedHandlersShareGlobalBudgetAcrossRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := options.NewRateLimitOptions()
	cfg.QueryGlobalQPS = 0.000001
	cfg.QueryGlobalBurst = 7
	cfg.QueryUserQPS = 1000
	cfg.QueryUserBurst = 1000

	router := newRateContractRouter(cfg)
	engine := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	engine.GET("/route-a", router.rateLimitedHandlers(
		rateLimitBudgetQuery,
		ok,
	)...)
	engine.GET("/route-b", router.rateLimitedHandlers(
		rateLimitBudgetQuery,
		ok,
	)...)

	const requests = 64
	start := make(chan struct{})
	var successes atomic.Int32
	var rejected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			path := "/route-a"
			if index%2 == 1 {
				path = "/route-b"
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			engine.ServeHTTP(recorder, request)
			switch recorder.Code {
			case http.StatusOK:
				successes.Add(1)
			case http.StatusTooManyRequests:
				rejected.Add(1)
			default:
				t.Errorf("status = %d, want 200 or 429", recorder.Code)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if got := successes.Load(); got != int32(cfg.QueryGlobalBurst) {
		t.Fatalf("successful requests = %d, want one shared burst of %d", got, cfg.QueryGlobalBurst)
	}
	if got := rejected.Load(); got != requests-int32(cfg.QueryGlobalBurst) {
		t.Fatalf("rejected requests = %d, want %d", got, requests-int32(cfg.QueryGlobalBurst))
	}
}

func TestRateLimitedHandlersSharePerUserBudgetAcrossRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := options.NewRateLimitOptions()
	cfg.QueryGlobalQPS = 1000
	cfg.QueryGlobalBurst = 1000
	cfg.QueryUserQPS = 0.000001
	cfg.QueryUserBurst = 5

	router := newRateContractRouter(cfg)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "42"})
		c.Next()
	})
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	engine.GET("/route-a", router.rateLimitedHandlers(
		rateLimitBudgetQuery,
		ok,
	)...)
	engine.GET("/route-b", router.rateLimitedHandlers(
		rateLimitBudgetQuery,
		ok,
	)...)

	const requests = 32
	start := make(chan struct{})
	var successes atomic.Int32
	var rejected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			path := "/route-a"
			if index%2 == 1 {
				path = "/route-b"
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			switch recorder.Code {
			case http.StatusOK:
				successes.Add(1)
			case http.StatusTooManyRequests:
				rejected.Add(1)
			default:
				t.Errorf("status = %d, want 200 or 429", recorder.Code)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if got := successes.Load(); got != int32(cfg.QueryUserBurst) {
		t.Fatalf("successful requests = %d, want one shared user burst of %d", got, cfg.QueryUserBurst)
	}
	if got := rejected.Load(); got != requests-int32(cfg.QueryUserBurst) {
		t.Fatalf("rejected requests = %d, want %d", got, requests-int32(cfg.QueryUserBurst))
	}
}

func TestRateLimitedHandlersKeepDifferentBudgetsIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := options.NewRateLimitOptions()
	cfg.QueryGlobalQPS = 0.000001
	cfg.QueryGlobalBurst = 1
	cfg.QueryUserQPS = 100
	cfg.QueryUserBurst = 100
	cfg.SubmitGlobalQPS = cfg.QueryGlobalQPS
	cfg.SubmitGlobalBurst = cfg.QueryGlobalBurst
	cfg.SubmitUserQPS = cfg.QueryUserQPS
	cfg.SubmitUserBurst = cfg.QueryUserBurst

	router := newRateContractRouter(cfg)
	engine := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	engine.GET("/query", router.rateLimitedHandlers(
		rateLimitBudgetQuery,
		ok,
	)...)
	engine.POST("/submit", router.rateLimitedHandlers(
		rateLimitBudgetSubmit,
		ok,
	)...)

	assertStatus := func(method, path string, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
		if recorder.Code != want {
			t.Fatalf("%s %s status = %d, want %d", method, path, recorder.Code, want)
		}
	}
	assertStatus(http.MethodGet, "/query", http.StatusOK)
	assertStatus(http.MethodPost, "/submit", http.StatusOK)
	assertStatus(http.MethodGet, "/query", http.StatusTooManyRequests)
}

func TestWaitReportBudgetUsesDedicatedConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := options.NewRateLimitOptions()
	cfg.QueryGlobalQPS = 100
	cfg.QueryGlobalBurst = 100
	cfg.WaitReportGlobalQPS = 0.000001
	cfg.WaitReportGlobalBurst = 1
	cfg.WaitReportUserQPS = 100
	cfg.WaitReportUserBurst = 100

	router := newRateContractRouter(cfg)
	engine := gin.New()
	engine.GET("/wait-report", router.rateLimitedHandlers(rateLimitBudgetWaitReport, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})...)

	for index, want := range []int{http.StatusOK, http.StatusTooManyRequests} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/wait-report", nil))
		if recorder.Code != want {
			t.Fatalf("request %d status = %d, want %d", index+1, recorder.Code, want)
		}
	}
}

func newRateContractRouter(cfg *options.RateLimitOptions) *Router {
	provider, err := resiliencesubsystem.New(resiliencesubsystem.Options{RateLimit: cfg})
	if err != nil {
		panic(err)
	}
	return NewRouter(Deps{RateLimit: cfg, RateBudgets: provider})
}

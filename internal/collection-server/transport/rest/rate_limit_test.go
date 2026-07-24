package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit/redisadapter"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	redis "github.com/redis/go-redis/v9"
)

func TestDistributedLimitFailOpenWhenDistributedLimiterUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	observer := &rateLimitRecordingObserver{}
	router := gin.New()
	router.GET("/", distributedLimitWithOptions(nil, "limit:submit:global", nil, pkgmiddleware.LimitOptions{Observer: observer}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if !observer.has(resilience.OutcomeDegradedOpen) {
		t.Fatal("expected degraded_open outcome")
	}
}

func TestDistributedLimitRejectsWithRetryAfterAndOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	limiter := ratelimit.NewDistributedLimiter(redisadapter.NewBackend(client, keyspace.NewBuilderWithNamespace("ops:runtime")), ratelimit.RateLimitPolicy{
		Component:     "collection-server",
		Scope:         "submit",
		Resource:      "global",
		Strategy:      "redis",
		RatePerSecond: 1,
		Burst:         1,
	})
	observer := &rateLimitRecordingObserver{}
	router := gin.New()
	router.GET("/", distributedLimitWithOptions(limiter, "limit:submit:global", nil, pkgmiddleware.LimitOptions{Observer: observer}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if got := recorder.Header().Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header is empty")
	}
	if !observer.has(resilience.OutcomeAllowed) {
		t.Fatal("expected allowed outcome")
	}
	if !observer.has(resilience.OutcomeRateLimited) {
		t.Fatal("expected rate_limited outcome")
	}
}

func TestDistributedLimitUsesLocalFallbackAndReturnsRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fallbackPolicy := ratelimit.RateLimitPolicy{
		Component: "collection-server", Scope: "submit", Resource: "global", Strategy: "local_fallback",
		RatePerSecond: 1, Burst: 1,
	}
	limiter := ratelimit.NewDegradedFallbackLimiter(
		rateLimiterFunc(func(context.Context, string) ratelimit.RateLimitDecision {
			return ratelimit.RateLimitDecision{
				Allowed: true, Outcome: resilience.OutcomeDegradedOpen,
				Subject: resilience.Subject{Component: "collection-server", Scope: "submit", Resource: "global", Strategy: "redis"},
			}
		}),
		ratelimit.NewLocalLimiter(fallbackPolicy),
	)
	observer := &rateLimitRecordingObserver{}
	router := gin.New()
	router.GET("/", distributedLimitWithOptions(limiter, "limit:submit:global", nil, pkgmiddleware.LimitOptions{Observer: observer}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	if first.Code != http.StatusNoContent || !observer.hasWithStrategy(resilience.OutcomeDegradedOpen, "local_fallback") {
		t.Fatalf("first status=%d decisions=%+v", first.Code, observer.decisions)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("fallback 429 Retry-After header is empty")
	}
	if !observer.hasWithStrategy(resilience.OutcomeRateLimited, "local_fallback") {
		t.Fatalf("fallback rate_limited decision missing: %+v", observer.decisions)
	}
}

type rateLimiterFunc func(context.Context, string) ratelimit.RateLimitDecision

func (f rateLimiterFunc) Decide(ctx context.Context, key string) ratelimit.RateLimitDecision {
	return f(ctx, key)
}

type rateLimitRecordingObserver struct {
	decisions []resilience.Decision
}

func (r *rateLimitRecordingObserver) ObserveDecision(_ context.Context, decision resilience.Decision) {
	r.decisions = append(r.decisions, decision)
}

func (r *rateLimitRecordingObserver) has(outcome resilience.Outcome) bool {
	for _, decision := range r.decisions {
		if decision.Outcome == outcome {
			return true
		}
	}
	return false
}

func (r *rateLimitRecordingObserver) hasWithStrategy(outcome resilience.Outcome, strategy string) bool {
	for _, decision := range r.decisions {
		if decision.Outcome == outcome && decision.Subject.Strategy == strategy {
			return true
		}
	}
	return false
}

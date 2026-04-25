package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	redis "github.com/redis/go-redis/v9"
)

func TestDistributedLimitFailOpenWhenRedisLimiterUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	observer := &rateLimitRecordingObserver{}
	router := gin.New()
	router.GET("/", distributedLimitWithOptions(nil, "limit:submit:global", 1, 1, nil, "submit", "global", pkgmiddleware.LimitOptions{Observer: observer}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if !observer.has(resilienceplane.OutcomeDegradedOpen) {
		t.Fatal("expected degraded_open outcome")
	}
}

func TestDistributedLimitRejectsWithRetryAfterAndOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	limiter := redisplane.NewDistributedLimiter(&redisplane.Handle{
		Family:  redisplane.FamilyOps,
		Client:  client,
		Builder: rediskey.NewBuilderWithNamespace("ops:runtime"),
	})
	observer := &rateLimitRecordingObserver{}
	router := gin.New()
	router.GET("/", distributedLimitWithOptions(limiter, "limit:submit:global", 1, 1, nil, "submit", "global", pkgmiddleware.LimitOptions{Observer: observer}), func(c *gin.Context) {
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
	if !observer.has(resilienceplane.OutcomeAllowed) {
		t.Fatal("expected allowed outcome")
	}
	if !observer.has(resilienceplane.OutcomeRateLimited) {
		t.Fatal("expected rate_limited outcome")
	}
}

type rateLimitRecordingObserver struct {
	decisions []resilienceplane.Decision
}

func (r *rateLimitRecordingObserver) ObserveDecision(_ context.Context, decision resilienceplane.Decision) {
	r.decisions = append(r.decisions, decision)
}

func (r *rateLimitRecordingObserver) has(outcome resilienceplane.Outcome) bool {
	for _, decision := range r.decisions {
		if decision.Outcome == outcome {
			return true
		}
	}
	return false
}

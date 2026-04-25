package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/gin-gonic/gin"
)

func TestLimitRejectsWithRetryAfterAndOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	observer := &limitRecordingObserver{}
	handler := LimitWithOptions(1, 1, LimitOptions{
		Component: "test",
		Scope:     "submit",
		Resource:  "global",
		Strategy:  "local",
		Observer:  observer,
	})

	router := gin.New()
	router.GET("/", handler, func(c *gin.Context) {
		c.Status(http.StatusOK)
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

func TestLimitByKeyIsIndependentPerKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := LimitByKeyWithOptions(1, 1, func(c *gin.Context) string {
		return c.GetHeader("X-Test-Key")
	}, LimitOptions{Observer: &limitRecordingObserver{}})

	router := gin.New()
	router.GET("/", handler, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	reqA1 := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA1.Header.Set("X-Test-Key", "a")
	router.ServeHTTP(httptest.NewRecorder(), reqA1)

	reqA2 := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA2.Header.Set("X-Test-Key", "a")
	recorderA2 := httptest.NewRecorder()
	router.ServeHTTP(recorderA2, reqA2)
	if recorderA2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request for key a status = %d, want 429", recorderA2.Code)
	}

	reqB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqB.Header.Set("X-Test-Key", "b")
	recorderB := httptest.NewRecorder()
	router.ServeHTTP(recorderB, reqB)
	if recorderB.Code != http.StatusOK {
		t.Fatalf("first request for key b status = %d, want 200", recorderB.Code)
	}
}

type limitRecordingObserver struct {
	decisions []resilienceplane.Decision
}

func (r *limitRecordingObserver) ObserveDecision(_ context.Context, decision resilienceplane.Decision) {
	r.decisions = append(r.decisions, decision)
}

func (r *limitRecordingObserver) has(outcome resilienceplane.Outcome) bool {
	for _, decision := range r.decisions {
		if decision.Outcome == outcome {
			return true
		}
	}
	return false
}

package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

func TestDecodeSubscribeFrame(t *testing.T) {
	frame, err := decodeFrame([]byte(`{"op":"subscribe","assessment_id":"123","kind":"personality","testee_id":"456"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if frame.Op != OpSubscribe || frame.AssessmentID != "123" || frame.Kind != "personality" || frame.TesteeID != "456" {
		t.Fatalf("unexpected frame: %+v", frame)
	}
}

func TestEncodeStatusFrame(t *testing.T) {
	payload, err := encodeFrame(outboundFrame{
		Op:   OpStatus,
		Data: map[string]any{"status": "interpreted"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected payload")
	}
}

func TestConnectionManagerLimits(t *testing.T) {
	mgr := newConnectionManager(1, 1)
	if !mgr.TryAcquire("1") {
		t.Fatal("expected first acquire to succeed")
	}
	if mgr.TryAcquire("1") {
		t.Fatal("expected per-testee limit to reject")
	}
	mgr.Release("1")
	if !mgr.TryAcquire("2") {
		t.Fatal("expected acquire after release")
	}
}

func TestSubscribeLimiterDisabledMeansNoAdmissionCheck(t *testing.T) {
	backend := &recordingRateLimitBackend{}
	limiter := newSubscribeLimiter(backend, &options.RateLimitOptions{
		Enabled:                 false,
		ReportEventsGlobalQPS:   10,
		ReportEventsGlobalBurst: 20,
		ReportEventsUserQPS:     2,
		ReportEventsUserBurst:   4,
	})

	if limiter != nil {
		t.Fatal("disabled report-events rate limit should not install a limiter")
	}
	if calls := backend.snapshot(); len(calls) != 0 {
		t.Fatalf("disabled limiter backend calls = %+v, want none", calls)
	}
}

func TestSubscribeLimiterUsesSharedGlobalAndPerUserBudgets(t *testing.T) {
	backend := &recordingRateLimitBackend{}
	limiter := newSubscribeLimiter(backend, &options.RateLimitOptions{
		Enabled:                 true,
		ReportEventsGlobalQPS:   10,
		ReportEventsGlobalBurst: 20,
		ReportEventsUserQPS:     2,
		ReportEventsUserBurst:   4,
	})
	if limiter == nil {
		t.Fatal("enabled report-events rate limit should install a limiter")
	}

	if decision := limiter.Decide(context.Background(), "user:42"); !decision.Allowed {
		t.Fatalf("first subscribe decision = %+v, want allowed", decision)
	}

	calls := backend.snapshot()
	if len(calls) != 2 {
		t.Fatalf("backend calls = %+v, want global and per-user checks", calls)
	}
	if got := calls[0]; got.key != "limit:report_events:global" || got.rate != 10 || got.burst != 20 {
		t.Fatalf("global call = %+v, want shared global budget", got)
	}
	if got := calls[1]; got.key != "limit:report_events:user:user:42" || got.rate != 2 || got.burst != 4 {
		t.Fatalf("user call = %+v, want per-user budget", got)
	}
}

func TestLocalSubscribeLimiterSharesGlobalBudgetAcrossUsers(t *testing.T) {
	limiter := newSubscribeLimiter(nil, &options.RateLimitOptions{
		Enabled:                 true,
		ReportEventsGlobalQPS:   1,
		ReportEventsGlobalBurst: 1,
		ReportEventsUserQPS:     100,
		ReportEventsUserBurst:   100,
	})

	if decision := limiter.Decide(context.Background(), "user:1"); !decision.Allowed {
		t.Fatalf("first user decision = %+v, want allowed", decision)
	}
	if decision := limiter.Decide(context.Background(), "user:2"); decision.Allowed {
		t.Fatalf("second user decision = %+v, want shared global budget rejection", decision)
	}
}

func TestLocalSubscribeLimiterKeepsIndependentPerUserBudgets(t *testing.T) {
	limiter := newSubscribeLimiter(nil, &options.RateLimitOptions{
		Enabled:                 true,
		ReportEventsGlobalQPS:   100,
		ReportEventsGlobalBurst: 100,
		ReportEventsUserQPS:     1,
		ReportEventsUserBurst:   1,
	})

	if decision := limiter.Decide(context.Background(), "user:1"); !decision.Allowed {
		t.Fatalf("first user decision = %+v, want allowed", decision)
	}
	if decision := limiter.Decide(context.Background(), "user:1"); decision.Allowed {
		t.Fatalf("duplicate user decision = %+v, want per-user rejection", decision)
	}
	if decision := limiter.Decide(context.Background(), "user:2"); !decision.Allowed {
		t.Fatalf("different user decision = %+v, want independent budget", decision)
	}
}

func TestSubscribeLimitKeyUsesAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/report-events", nil)
	c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "42"})

	if got := subscribeLimitKey(c); got != "user:42" {
		t.Fatalf("subscribe limit key = %q, want authenticated user", got)
	}
}

type rateLimitBackendCall struct {
	key   string
	rate  float64
	burst int
}

type recordingRateLimitBackend struct {
	mu    sync.Mutex
	calls []rateLimitBackendCall
}

func (b *recordingRateLimitBackend) Allow(_ context.Context, key string, rate float64, burst int) (bool, time.Duration, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, rateLimitBackendCall{key: key, rate: rate, burst: burst})
	return true, 0, nil
}

func (b *recordingRateLimitBackend) snapshot() []rateLimitBackendCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]rateLimitBackendCall(nil), b.calls...)
}

package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportevents"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportnotify"
	appreportstatus "github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/collection-server/resilience/subsystem"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	sharedreportstatus "github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

func mustNewResilience(opts resiliencesubsystem.Options) *resiliencesubsystem.Subsystem {
	s, err := resiliencesubsystem.New(opts)
	if err != nil {
		panic(err)
	}
	return s
}

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
	cfg := &options.RateLimitOptions{
		Enabled:                 true,
		ReportEventsGlobalQPS:   10,
		ReportEventsGlobalBurst: 20,
		ReportEventsUserQPS:     2,
		ReportEventsUserBurst:   4,
	}
	limiter := newSubscribeLimiter(backend, cfg, mustNewResilience(resiliencesubsystem.Options{RateLimit: cfg, Backend: backend}))
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
	cfg := &options.RateLimitOptions{
		Enabled:                 true,
		ReportEventsGlobalQPS:   1,
		ReportEventsGlobalBurst: 1,
		ReportEventsUserQPS:     100,
		ReportEventsUserBurst:   100,
	}
	limiter := newSubscribeLimiter(nil, cfg, mustNewResilience(resiliencesubsystem.Options{RateLimit: cfg}))

	if decision := limiter.Decide(context.Background(), "user:1"); !decision.Allowed {
		t.Fatalf("first user decision = %+v, want allowed", decision)
	}
	if decision := limiter.Decide(context.Background(), "user:2"); decision.Allowed {
		t.Fatalf("second user decision = %+v, want shared global budget rejection", decision)
	}
}

func TestLocalSubscribeLimiterKeepsIndependentPerUserBudgets(t *testing.T) {
	cfg := &options.RateLimitOptions{
		Enabled:                 true,
		ReportEventsGlobalQPS:   100,
		ReportEventsGlobalBurst: 100,
		ReportEventsUserQPS:     1,
		ReportEventsUserBurst:   1,
	}
	limiter := newSubscribeLimiter(nil, cfg, mustNewResilience(resiliencesubsystem.Options{RateLimit: cfg}))

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

type wsTesteeAccess struct {
	mu      sync.Mutex
	results []error
	calls   int
}

func (a *wsTesteeAccess) Authorize(context.Context, string, uint64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	if len(a.results) == 0 {
		return nil
	}
	index := a.calls - 1
	if index >= len(a.results) {
		index = len(a.results) - 1
	}
	return a.results[index]
}

type wsKindReader struct {
	authorizeErr error
	status       *appreportstatus.View
}

func (r wsKindReader) Authorize(context.Context, uint64, uint64) error {
	return r.authorizeErr
}

func (r wsKindReader) CurrentStatus(context.Context, uint64, uint64) (*appreportstatus.View, error) {
	return r.status, nil
}

func newWSTestHandler(access reportevents.TesteeAccessAuthorizer, notifier reportnotify.Notifier, readers map[string]appreportstatus.KindReader) *ReportEventsHandler {
	opts := options.NewReportEventsOptions()
	opts.Enabled = true
	opts.HeartbeatIntervalSeconds = 60
	opts.IdleTimeoutSeconds = 120
	return NewReportEventsHandler(Dependencies{
		Notifier: notifier,
		Events: reportevents.NewService(
			access,
			appreportstatus.NewResolver(readers),
		),
		Options:      opts,
		RateLimitCfg: &options.RateLimitOptions{Enabled: false},
	})
}

func newWSTestServer(handler *ReportEventsHandler, authenticated bool) *httptest.Server {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/v1/report-events", func(c *gin.Context) {
		if authenticated {
			c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "user-7"})
		}
		handler.ServeHTTP(c)
	})
	return httptest.NewServer(engine)
}

func dialWSTest(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/report-events"
	conn, response, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if response != nil {
			t.Fatalf("dial websocket: %v (http %d)", err, response.StatusCode)
		}
		t.Fatalf("dial websocket: %v", err)
	}
	return conn
}

func writeWSFrame(t *testing.T, conn *websocket.Conn, frame inboundFrame) {
	t.Helper()
	payload, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, payload); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

func readWSFrame(t *testing.T, conn *websocket.Conn) outboundFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var frame outboundFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		t.Fatalf("unmarshal frame: %v", err)
	}
	return frame
}

func TestReportEventsRejectsMissingJWTBeforeUpgrade(t *testing.T) {
	handler := newWSTestHandler(&wsTesteeAccess{}, reportnotify.NewInMemoryNotifier(), nil)
	server := newWSTestServer(handler, false)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/report-events"
	conn, response, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "test complete")
		t.Fatal("expected websocket upgrade to be rejected")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("http response = %#v, want 401", response)
	}
}

func TestReportEventsDeniesAccessWithoutSubscriptionOrDetailLeak(t *testing.T) {
	tests := []struct {
		name       string
		access     reportevents.TesteeAccessAuthorizer
		reader     appreportstatus.KindReader
		kind       string
		wantCode   string
		wantReason string
	}{
		{
			name:       "foreign testee",
			access:     &wsTesteeAccess{results: []error{testeeaccess.ErrAccessDenied}},
			reader:     wsKindReader{},
			wantCode:   "forbidden",
			wantReason: "assessment access denied",
		},
		{
			name:       "foreign assessment",
			access:     &wsTesteeAccess{},
			reader:     wsKindReader{authorizeErr: appreportstatus.ErrAssessmentAccess},
			wantCode:   "forbidden",
			wantReason: "assessment access denied",
		},
		{
			name:       "nonexistent assessment",
			access:     &wsTesteeAccess{},
			reader:     wsKindReader{authorizeErr: appreportstatus.ErrAssessmentAccess},
			wantCode:   "forbidden",
			wantReason: "assessment access denied",
		},
		{
			name:       "authorization unavailable",
			access:     &wsTesteeAccess{results: []error{errors.New("secret iam endpoint failed")}},
			reader:     wsKindReader{},
			wantCode:   "temporarily_unavailable",
			wantReason: "authorization temporarily unavailable",
		},
		{
			name:       "invalid kind",
			access:     &wsTesteeAccess{},
			reader:     wsKindReader{},
			kind:       "unknown",
			wantCode:   "invalid_kind",
			wantReason: "invalid assessment kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := reportnotify.NewInMemoryNotifier()
			handler := newWSTestHandler(tt.access, notifier, map[string]appreportstatus.KindReader{
				appreportstatus.KindMedical: tt.reader,
			})
			server := newWSTestServer(handler, true)
			defer server.Close()
			conn := dialWSTest(t, server)
			defer func() {
				if err := conn.CloseNow(); err != nil {
					t.Fatalf("close websocket: %v", err)
				}
			}()

			kind := tt.kind
			if kind == "" {
				kind = appreportstatus.KindMedical
			}
			writeWSFrame(t, conn, inboundFrame{Op: OpSubscribe, AssessmentID: "42", TesteeID: "7", Kind: kind})
			frame := readWSFrame(t, conn)
			if frame.Op != OpError || frame.Code != tt.wantCode || frame.Message != tt.wantReason {
				t.Fatalf("frame = %+v", frame)
			}
			if strings.Contains(frame.Message, "secret") {
				t.Fatalf("frame leaks dependency detail: %+v", frame)
			}
			if notifier.ActiveSubscriptions() != 0 {
				t.Fatalf("active subscriptions = %d, want 0", notifier.ActiveSubscriptions())
			}
			handler.connMgr.mu.Lock()
			activeConnections := handler.connMgr.total
			handler.connMgr.mu.Unlock()
			if activeConnections != 0 {
				t.Fatalf("active connections = %d, want 0", activeConnections)
			}
		})
	}
}

func TestReportEventsAllowsAllSupportedKinds(t *testing.T) {
	for _, kind := range []string{
		appreportstatus.KindMedical,
		appreportstatus.KindPersonality,
		appreportstatus.KindBehavior,
	} {
		t.Run(kind, func(t *testing.T) {
			notifier := reportnotify.NewInMemoryNotifier()
			handler := newWSTestHandler(&wsTesteeAccess{}, notifier, map[string]appreportstatus.KindReader{
				kind: wsKindReader{status: &appreportstatus.View{Status: "interpreted", Stage: "completed", UpdatedAt: 1}},
			})
			server := newWSTestServer(handler, true)
			defer server.Close()
			conn := dialWSTest(t, server)
			defer func() {
				if err := conn.CloseNow(); err != nil {
					t.Fatalf("close websocket: %v", err)
				}
			}()

			writeWSFrame(t, conn, inboundFrame{Op: OpSubscribe, AssessmentID: "42", TesteeID: "7", Kind: kind})
			if frame := readWSFrame(t, conn); frame.Op != OpSubscribed {
				t.Fatalf("subscribed frame = %+v", frame)
			}
			if frame := readWSFrame(t, conn); frame.Op != OpStatus {
				t.Fatalf("status frame = %+v", frame)
			}
		})
	}
}

func TestReportEventsRechecksTesteeAccessAfterSignal(t *testing.T) {
	notifier := reportnotify.NewInMemoryNotifier()
	access := &wsTesteeAccess{results: []error{nil, testeeaccess.ErrAccessDenied}}
	handler := newWSTestHandler(access, notifier, map[string]appreportstatus.KindReader{
		appreportstatus.KindMedical: wsKindReader{status: &appreportstatus.View{Status: "processing", Stage: "processing", UpdatedAt: 1}},
	})
	server := newWSTestServer(handler, true)
	defer server.Close()
	conn := dialWSTest(t, server)
	defer func() {
		// Server already closes with StatusPolicyViolation after access revocation.
		_ = conn.CloseNow()
	}()

	writeWSFrame(t, conn, inboundFrame{Op: OpSubscribe, AssessmentID: "42", TesteeID: "7", Kind: appreportstatus.KindMedical})
	if frame := readWSFrame(t, conn); frame.Op != OpSubscribed {
		t.Fatalf("subscribed frame = %+v", frame)
	}
	if frame := readWSFrame(t, conn); frame.Op != OpStatus {
		t.Fatalf("initial status frame = %+v", frame)
	}

	notifier.Notify(sharedreportstatus.ChangedSignal{AssessmentID: "42", Status: "completed", OccurredAt: time.Now()})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, payload, err := conn.Read(ctx)
	if err == nil {
		t.Fatalf("unexpected frame after access revocation: %s", payload)
	}
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("close status = %d, want %d (err=%v)", websocket.CloseStatus(err), websocket.StatusPolicyViolation, err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		handler.connMgr.mu.Lock()
		activeConnections := handler.connMgr.total
		handler.connMgr.mu.Unlock()
		if activeConnections == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("active connections = %d, want 0", activeConnections)
		}
		time.Sleep(time.Millisecond)
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

package ws

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportevents"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportnotify"
	appreportstatus "github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

// Dependencies wires report-events WebSocket transport.
type Dependencies struct {
	Notifier     reportnotify.Notifier
	Events       *reportevents.Service
	Options      *options.ReportEventsOptions
	RateLimit    ratelimit.Backend
	RateLimitCfg *options.RateLimitOptions
	RateBudgets  ratelimit.RateBudgetProvider
}

// ReportEventsHandler serves WSS /api/v1/report-events.
type ReportEventsHandler struct {
	notifier reportnotify.Notifier
	events   *reportevents.Service
	opts     *options.ReportEventsOptions
	connMgr  *connectionManager
	limiter  ratelimit.RateLimiter
}

// ginWebsocketResponseWriter lets coder/websocket flush Gin's 101 response
// bookkeeping while bypassing Gin's "already written" guard during hijack.
type ginWebsocketResponseWriter struct {
	gin.ResponseWriter
	underlying http.ResponseWriter
}

func (w *ginWebsocketResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.underlying.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

type subscribeLimiter struct {
	global ratelimit.RateLimiter
	user   ratelimit.RateLimiter
}

func NewReportEventsHandler(deps Dependencies) *ReportEventsHandler {
	opts := deps.Options
	if opts == nil {
		opts = options.NewReportEventsOptions()
	}
	return &ReportEventsHandler{
		notifier: deps.Notifier,
		events:   deps.Events,
		opts:     opts,
		connMgr:  newConnectionManager(opts.MaxConnections, opts.MaxPerTestee),
		limiter:  newSubscribeLimiter(deps.RateLimit, deps.RateLimitCfg, deps.RateBudgets),
	}
}

func newSubscribeLimiter(_ ratelimit.Backend, cfg *options.RateLimitOptions, providers ...ratelimit.RateBudgetProvider) ratelimit.RateLimiter {
	if cfg == nil {
		cfg = options.NewRateLimitOptions()
	}
	if !cfg.Enabled {
		return nil
	}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if budget, ok := provider.Budget(ratelimit.BudgetID("report_events")); ok {
			return &subscribeLimiter{global: budget.Global, user: budget.User}
		}
	}

	return unavailableSubscribeLimiter{}
}

type unavailableSubscribeLimiter struct{}

func (unavailableSubscribeLimiter) Decide(context.Context, string) ratelimit.RateLimitDecision {
	return ratelimit.RateLimitDecision{Allowed: false, RetryAfter: time.Second, RetryAfterSeconds: 1}
}

func (l *subscribeLimiter) Decide(ctx context.Context, key string) ratelimit.RateLimitDecision {
	globalDecision := l.global.Decide(ctx, "limit:report_events:global")
	if !globalDecision.Allowed || l.user == nil {
		return globalDecision
	}
	return l.user.Decide(ctx, "limit:report_events:user:"+key)
}

func subscribeLimitKey(c *gin.Context) string {
	if userID := pkgmiddleware.GetUserID(c); userID != "" {
		return "user:" + userID
	}
	return "ip:" + c.ClientIP()
}

func (h *ReportEventsHandler) Enabled() bool {
	return h != nil && h.opts != nil && h.opts.Enabled
}

func (h *ReportEventsHandler) Path() string {
	if h == nil || h.opts == nil || h.opts.Path == "" {
		return "/api/v1/report-events"
	}
	return h.opts.Path
}

func (h *ReportEventsHandler) ServeHTTP(c *gin.Context) {
	if !h.Enabled() {
		c.Status(http.StatusNotFound)
		return
	}
	userID := pkgmiddleware.GetUserID(c)
	if userID == "" {
		incSubscribeDenied("unauthenticated")
		c.JSON(http.StatusUnauthorized, gin.H{"message": "authentication required"})
		return
	}
	responseWriter := http.ResponseWriter(c.Writer)
	if unwrapper, ok := responseWriter.(interface{ Unwrap() http.ResponseWriter }); ok {
		responseWriter = &ginWebsocketResponseWriter{
			ResponseWriter: c.Writer,
			underlying:     unwrapper.Unwrap(),
		}
	}
	conn, err := websocket.Accept(responseWriter, c.Request, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "closed") }()

	ctx, cancelConnection := context.WithCancel(context.WithoutCancel(c.Request.Context()))
	defer cancelConnection()
	limitKey := subscribeLimitKey(c)
	idleTimeout := time.Duration(h.opts.IdleTimeoutSeconds) * time.Second
	heartbeat := time.Duration(h.opts.HeartbeatIntervalSeconds) * time.Second

	var (
		mu           sync.Mutex
		activeTestee string
		subscribed   bool
		unsubscribe  func()
		lastActive   = time.Now()
	)
	defer func() {
		if unsubscribe != nil {
			unsubscribe()
		}
		if activeTestee != "" {
			h.connMgr.Release(activeTestee)
		}
	}()

	touch := func() {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()
	}

	writer := newFrameWriter(conn)

	go func() {
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				idle := time.Since(lastActive)
				mu.Unlock()
				if idle >= idleTimeout {
					_ = conn.Close(websocket.StatusPolicyViolation, "idle timeout")
					return
				}
				if err := writer.write(ctx, outboundFrame{Op: OpPong}); err != nil {
					return
				}
			}
		}
	}()

	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			return
		}
		touch()

		frame, err := decodeFrame(payload)
		if err != nil {
			_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "bad_request", Message: "invalid frame"})
			continue
		}

		switch frame.Op {
		case OpPing:
			if err := writer.write(ctx, outboundFrame{Op: OpPong}); err != nil {
				return
			}
		case OpSubscribe:
			if subscribed {
				_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "already_subscribed", Message: "only one active subscription per connection"})
				continue
			}
			if h.limiter != nil {
				decision := h.limiter.Decide(ctx, limitKey)
				if !decision.Allowed {
					incSubscribeDenied("rate_limited")
					_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "rate_limited", Message: "subscribe rate limited"})
					continue
				}
			}
			if !h.connMgr.TryAcquire(frame.TesteeID) {
				_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "capacity_exhausted", Message: "connection capacity exhausted"})
				continue
			}
			activeTestee = frame.TesteeID

			assessmentID, err := reportevents.ParseUintID(frame.AssessmentID)
			if err != nil {
				h.connMgr.Release(activeTestee)
				activeTestee = ""
				_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "bad_request", Message: "invalid assessment_id"})
				continue
			}
			testeeID, err := reportevents.ParseUintID(frame.TesteeID)
			if err != nil {
				h.connMgr.Release(activeTestee)
				activeTestee = ""
				_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "bad_request", Message: "invalid testee_id"})
				continue
			}

			var status *reportevents.StatusPayload
			if h.events == nil {
				err = testeeaccess.ErrAccessUnavailable
			} else {
				status, err = h.events.CurrentStatus(ctx, userID, frame.Kind, testeeID, assessmentID)
			}
			if err != nil {
				h.connMgr.Release(activeTestee)
				activeTestee = ""
				code, message, reason := reportEventsAccessError(err)
				incSubscribeDenied(reason)
				log.Warnw("report-events subscribe denied",
					"request_id", pkgmiddleware.RequestIDFromStandardContext(ctx),
					"user_id", userID,
					"testee_id", testeeID,
					"assessment_id", assessmentID,
					"kind", frame.Kind,
					"reason", reason,
					"error", err,
				)
				_ = writer.write(ctx, outboundFrame{Op: OpError, Code: code, Message: message})
				continue
			}
			if h.notifier == nil {
				h.connMgr.Release(activeTestee)
				activeTestee = ""
				incSubscribeDenied("authorization_unavailable")
				_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "temporarily_unavailable", Message: "authorization temporarily unavailable"})
				continue
			}

			signalCh, cancel := h.notifier.Subscribe(strconv.FormatUint(assessmentID, 10))
			unsubscribe = cancel
			subscribed = true

			if err := writer.write(ctx, outboundFrame{
				Op:           OpSubscribed,
				AssessmentID: frame.AssessmentID,
			}); err != nil {
				return
			}
			if err := h.pushStatus(ctx, writer, status); err != nil {
				return
			}
			if appreportstatus.IsTerminalStatus(status.Status) {
				return
			}

			go h.forwardSignals(ctx, writer, userID, frame.Kind, testeeID, assessmentID, signalCh, touch)
		default:
			_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "bad_request", Message: fmt.Sprintf("unsupported op %q", frame.Op)})
		}
	}
}

func (h *ReportEventsHandler) forwardSignals(
	ctx context.Context,
	writer *frameWriter,
	userID string,
	kind string,
	testeeID, assessmentID uint64,
	signalCh <-chan reportnotify.StatusEvent,
	touch func(),
) {
	for {
		select {
		case <-ctx.Done():
			return
		case signal, ok := <-signalCh:
			if !ok {
				return
			}
			if signal.Status != "completed" && signal.Status != "failed" {
				continue
			}
			status, err := h.events.CurrentStatus(ctx, userID, kind, testeeID, assessmentID)
			if err != nil {
				code, _, reason := reportEventsAccessError(err)
				log.Warnw("report-events subscription authorization revoked",
					"request_id", pkgmiddleware.RequestIDFromStandardContext(ctx),
					"user_id", userID,
					"testee_id", testeeID,
					"assessment_id", assessmentID,
					"kind", kind,
					"reason", reason,
					"error", err,
				)
				closeReason := "temporarily unavailable"
				if code == "forbidden" {
					closeReason = "assessment access denied"
				}
				_ = writer.conn.Close(websocket.StatusPolicyViolation, closeReason)
				return
			}
			touch()
			if err := h.pushStatus(ctx, writer, status); err != nil {
				return
			}
			if appreportstatus.IsTerminalStatus(status.Status) {
				_ = writer.conn.Close(websocket.StatusNormalClosure, "completed")
				return
			}
		}
	}
}

func (h *ReportEventsHandler) pushStatus(ctx context.Context, writer *frameWriter, status *reportevents.StatusPayload) error {
	incPush()
	return writer.write(ctx, outboundFrame{Op: OpStatus, Data: status})
}

func reportEventsAccessError(err error) (code, message, reason string) {
	switch {
	case errors.Is(err, appreportstatus.ErrInvalidKind):
		return "invalid_kind", "invalid assessment kind", "invalid_kind"
	case errors.Is(err, testeeaccess.ErrAccessDenied):
		return "forbidden", "assessment access denied", "testee_access"
	case errors.Is(err, appreportstatus.ErrAssessmentAccess):
		return "forbidden", "assessment access denied", "assessment_access"
	default:
		return "temporarily_unavailable", "authorization temporarily unavailable", "authorization_unavailable"
	}
}

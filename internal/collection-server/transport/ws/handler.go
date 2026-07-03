package ws

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportevents"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportnotify"
	appreportstatus "github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
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
}

// ReportEventsHandler serves WSS /api/v1/report-events.
type ReportEventsHandler struct {
	notifier reportnotify.Notifier
	events   *reportevents.Service
	opts     *options.ReportEventsOptions
	connMgr  *connectionManager
	limiter  ratelimit.RateLimiter
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
		limiter:  newSubscribeLimiter(deps.RateLimit, deps.RateLimitCfg),
	}
}

func newSubscribeLimiter(backend ratelimit.Backend, cfg *options.RateLimitOptions) ratelimit.RateLimiter {
	if cfg == nil {
		cfg = options.NewRateLimitOptions()
	}
	policy := ratelimit.RateLimitPolicy{
		Component:     "collection-server",
		Scope:         "report_events",
		Resource:      "subscribe",
		Strategy:      "local",
		RatePerSecond: cfg.ReportEventsGlobalQPS,
		Burst:         cfg.ReportEventsGlobalBurst,
	}
	if backend != nil {
		policy.Strategy = "redis"
		return ratelimit.NewDistributedLimiter(backend, policy)
	}
	return ratelimit.NewLocalLimiter(policy)
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
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "closed") }()

	ctx := c.Request.Context()
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
				decision := h.limiter.Decide(ctx, frame.TesteeID)
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

			status, err := h.events.CurrentStatus(ctx, frame.Kind, testeeID, assessmentID)
			if err != nil {
				h.connMgr.Release(activeTestee)
				activeTestee = ""
				code := "forbidden"
				if errors.Is(err, reportevents.ErrInvalidKind) {
					code = "invalid_kind"
				} else if errors.Is(err, reportevents.ErrAssessmentAccess) {
					code = "forbidden"
				}
				incSubscribeDenied(code)
				_ = writer.write(ctx, outboundFrame{Op: OpError, Code: code, Message: err.Error()})
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

			go h.forwardSignals(ctx, writer, frame.Kind, testeeID, assessmentID, signalCh, touch)
		default:
			_ = writer.write(ctx, outboundFrame{Op: OpError, Code: "bad_request", Message: fmt.Sprintf("unsupported op %q", frame.Op)})
		}
	}
}

func (h *ReportEventsHandler) forwardSignals(
	ctx context.Context,
	writer *frameWriter,
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
			status, err := h.events.CurrentStatus(ctx, kind, testeeID, assessmentID)
			if err != nil {
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

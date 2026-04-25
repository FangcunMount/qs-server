package backpressure

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// Limiter provides a simple in-flight limiter with optional timeout.
type Limiter struct {
	sem      chan struct{}
	timeout  time.Duration
	subject  resilienceplane.Subject
	observer resilienceplane.Observer
}

type Options struct {
	Component  string
	Dependency string
	Observer   resilienceplane.Observer
}

// NewLimiter creates a limiter with maxInflight and optional timeout.
func NewLimiter(maxInflight int, timeout time.Duration) *Limiter {
	return NewLimiterWithOptions(maxInflight, timeout, Options{})
}

func NewLimiterWithOptions(maxInflight int, timeout time.Duration, opts Options) *Limiter {
	if maxInflight <= 0 {
		return nil
	}
	return &Limiter{
		sem:     make(chan struct{}, maxInflight),
		timeout: timeout,
		subject: resilienceplane.Subject{
			Component: opts.Component,
			Scope:     opts.Dependency,
			Resource:  "downstream",
			Strategy:  "semaphore",
		},
		observer: defaultObserver(opts.Observer),
	}
}

// Acquire waits for a slot or until context timeout/cancel.
// It returns a possibly wrapped context and a release function.
func (l *Limiter) Acquire(ctx context.Context) (context.Context, func(), error) {
	if l == nil {
		return ctx, func() {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	waitCtx := ctx
	var cancel context.CancelFunc
	if l.timeout > 0 {
		if deadline, ok := waitCtx.Deadline(); !ok || time.Until(deadline) > l.timeout {
			waitCtx, cancel = context.WithTimeout(waitCtx, l.timeout)
		}
	}

	select {
	case l.sem <- struct{}{}:
		l.observe(ctx, resilienceplane.OutcomeBackpressureAcquired)
		release := func() {
			<-l.sem
			l.observe(ctx, resilienceplane.OutcomeBackpressureReleased)
			if cancel != nil {
				cancel()
			}
		}
		// Preserve the original request context for the downstream operation.
		// The limiter timeout only applies to waiting for a slot, not to the work itself.
		return ctx, release, nil
	case <-waitCtx.Done():
		if cancel != nil {
			cancel()
		}
		l.observe(ctx, resilienceplane.OutcomeBackpressureTimeout)
		return ctx, func() {}, waitCtx.Err()
	}
}

func (l *Limiter) observe(ctx context.Context, outcome resilienceplane.Outcome) {
	if l == nil {
		return
	}
	resilienceplane.Observe(ctx, l.observer, resilienceplane.ProtectionBackpressure, l.subject, outcome)
}

func defaultObserver(observer resilienceplane.Observer) resilienceplane.Observer {
	if observer != nil {
		return observer
	}
	return resilienceplane.DefaultObserver()
}

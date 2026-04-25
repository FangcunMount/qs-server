package backpressure

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// Limiter provides a simple in-flight limiter with optional timeout.
type Limiter struct {
	sem         chan struct{}
	maxInflight int
	timeout     time.Duration
	subject     resilienceplane.Subject
	observer    resilienceplane.Observer
}

type Acquirer interface {
	Acquire(context.Context) (context.Context, func(), error)
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
		sem:         make(chan struct{}, maxInflight),
		maxInflight: maxInflight,
		timeout:     timeout,
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

	waitStartedAt := time.Now()
	waitCtx := ctx
	var cancel context.CancelFunc
	if l.timeout > 0 {
		if deadline, ok := waitCtx.Deadline(); !ok || time.Until(deadline) > l.timeout {
			waitCtx, cancel = context.WithTimeout(waitCtx, l.timeout)
		}
	}

	select {
	case l.sem <- struct{}{}:
		l.observeWait(resilienceplane.OutcomeBackpressureAcquired, time.Since(waitStartedAt))
		l.observe(ctx, resilienceplane.OutcomeBackpressureAcquired)
		l.observeInFlight()
		release := func() {
			<-l.sem
			l.observe(ctx, resilienceplane.OutcomeBackpressureReleased)
			l.observeInFlight()
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
		l.observeWait(resilienceplane.OutcomeBackpressureTimeout, time.Since(waitStartedAt))
		l.observe(ctx, resilienceplane.OutcomeBackpressureTimeout)
		return ctx, func() {}, waitCtx.Err()
	}
}

func (l *Limiter) Snapshot(name string) resilienceplane.BackpressureSnapshot {
	if l == nil {
		return resilienceplane.BackpressureSnapshot{
			Name:     name,
			Enabled:  false,
			Degraded: true,
			Reason:   "backpressure limiter disabled",
		}
	}
	subject := l.subject
	if name == "" {
		name = subject.Scope
	}
	return resilienceplane.BackpressureSnapshot{
		Component:     subject.Component,
		Name:          name,
		Dependency:    subject.Scope,
		Strategy:      subject.Strategy,
		Enabled:       true,
		MaxInflight:   l.maxInflight,
		InFlight:      len(l.sem),
		TimeoutMillis: l.timeout.Milliseconds(),
	}
}

func (l *Limiter) observe(ctx context.Context, outcome resilienceplane.Outcome) {
	if l == nil {
		return
	}
	resilienceplane.Observe(ctx, l.observer, resilienceplane.ProtectionBackpressure, l.subject, outcome)
}

func (l *Limiter) observeInFlight() {
	if l == nil {
		return
	}
	resilienceplane.ObserveBackpressureInFlight(l.subject, len(l.sem))
}

func (l *Limiter) observeWait(outcome resilienceplane.Outcome, duration time.Duration) {
	if l == nil {
		return
	}
	resilienceplane.ObserveBackpressureWaitDuration(l.subject, outcome, duration)
}

func defaultObserver(observer resilienceplane.Observer) resilienceplane.Observer {
	if observer != nil {
		return observer
	}
	return resilienceplane.DefaultObserver()
}

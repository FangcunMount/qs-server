package backpressure

import (
	"context"
	"time"
)

// Limiter provides a simple in-flight limiter with optional timeout.
type Limiter struct {
	sem     chan struct{}
	timeout time.Duration
}

// NewLimiter creates a limiter with maxInflight and optional timeout.
func NewLimiter(maxInflight int, timeout time.Duration) *Limiter {
	if maxInflight <= 0 {
		return nil
	}
	return &Limiter{
		sem:     make(chan struct{}, maxInflight),
		timeout: timeout,
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
		release := func() {
			<-l.sem
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
		return ctx, func() {}, waitCtx.Err()
	}
}

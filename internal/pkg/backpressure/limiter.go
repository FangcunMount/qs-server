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

	var cancel context.CancelFunc
	if l.timeout > 0 {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > l.timeout {
			ctx, cancel = context.WithTimeout(ctx, l.timeout)
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
		return ctx, release, nil
	case <-ctx.Done():
		if cancel != nil {
			cancel()
		}
		return ctx, func() {}, ctx.Err()
	}
}

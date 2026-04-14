package backpressure

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAcquireDoesNotWrapOperationContextWithLimiterTimeout(t *testing.T) {
	limiter := NewLimiter(1, 50*time.Millisecond)

	ctx := context.Background()
	gotCtx, release, err := limiter.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer release()

	if gotCtx != ctx {
		t.Fatalf("Acquire() should preserve original context")
	}
	if _, ok := gotCtx.Deadline(); ok {
		t.Fatalf("Acquire() should not add a deadline to the downstream operation context")
	}
}

func TestAcquireTimeoutOnlyAppliesWhileWaitingForSlot(t *testing.T) {
	limiter := NewLimiter(1, 50*time.Millisecond)

	_, release, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer release()

	_, _, err = limiter.Acquire(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

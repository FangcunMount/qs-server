package eventoutbox

import (
	"context"
	"testing"
)

type limiterSpy struct {
	acquires int
}

func (l *limiterSpy) Acquire(ctx context.Context) (context.Context, func(), error) {
	l.acquires++
	return ctx, func() {}, nil
}

func TestWithLimiterSetsStoreLimiter(t *testing.T) {
	spy := &limiterSpy{}
	store := &Store{}
	WithLimiter(spy)(store)
	if store.limiter != spy {
		t.Fatalf("limiter = %#v, want spy", store.limiter)
	}
}

func TestAcquireWithoutLimiterIsNoop(t *testing.T) {
	store := &Store{}
	ctx, release, err := store.acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire() error = %v", err)
	}
	release()
	if ctx == nil {
		t.Fatal("acquire() returned nil context")
	}
}

func TestAcquireUsesConfiguredLimiter(t *testing.T) {
	spy := &limiterSpy{}
	store := &Store{limiter: spy}

	_, release, err := store.acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire() error = %v", err)
	}
	release()

	if spy.acquires != 1 {
		t.Fatalf("acquires = %d, want 1", spy.acquires)
	}
}

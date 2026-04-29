package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
)

type fakeBehaviorProjector struct {
	mu      sync.Mutex
	limits  []int
	err     error
	started chan int
	block   <-chan struct{}
}

func (f *fakeBehaviorProjector) ProjectBehaviorEvent(context.Context, statisticsApp.BehaviorProjectEventInput) (statisticsApp.BehaviorProjectEventResult, error) {
	return statisticsApp.BehaviorProjectEventResult{Status: statisticsApp.BehaviorProjectEventStatusCompleted}, nil
}

func (f *fakeBehaviorProjector) ReconcilePendingBehaviorEvents(ctx context.Context, limit int) (int, error) {
	f.mu.Lock()
	f.limits = append(f.limits, limit)
	err := f.err
	started := f.started
	block := f.block
	f.mu.Unlock()

	if started != nil {
		select {
		case started <- limit:
		default:
		}
	}
	if block != nil {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-block:
		}
	}
	if err != nil {
		return 0, err
	}
	return limit, nil
}

func (f *fakeBehaviorProjector) calls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()

	limits := make([]int, len(f.limits))
	copy(limits, f.limits)
	return limits
}

func TestNewBehaviorPendingReconcileRunner(t *testing.T) {
	opts := newTestBehaviorPendingReconcileOptions()
	projector := &fakeBehaviorProjector{}

	if runner := newBehaviorPendingReconcileRunnerWithHooks(
		&apiserveroptions.BehaviorPendingReconcileOptions{Enable: false},
		projector,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected disabled reconcile runner to return nil")
	}

	if runner := newBehaviorPendingReconcileRunnerWithHooks(
		opts,
		nil,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil projector to return nil")
	}

	if runner := newBehaviorPendingReconcileRunnerWithHooks(
		opts,
		projector,
		nil,
		newTestBehaviorLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil lock manager to return nil")
	}

	invalid := newTestBehaviorPendingReconcileOptions()
	invalid.Interval = 0
	if runner := newBehaviorPendingReconcileRunnerWithHooks(
		invalid,
		projector,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected invalid interval to return nil")
	}
}

func TestBehaviorPendingReconcileLockKeyUsesLockNamespace(t *testing.T) {
	runner := newBehaviorPendingReconcileRunnerWithHooks(
		newTestBehaviorPendingReconcileOptions(),
		&fakeBehaviorProjector{},
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	)
	if got := runner.lockKey(); got != "apiserver-test:cache:lock:qs:behavior-pending-reconcile:test" {
		t.Fatalf("unexpected lock key: %s", got)
	}
}

func TestBehaviorPendingReconcileRunOnceUsesConfiguredLockOverride(t *testing.T) {
	projector := &fakeBehaviorProjector{}
	opts := newTestBehaviorPendingReconcileOptions()
	opts.LockKey = "qs:behavior-pending-reconcile:custom"
	opts.LockTTL = 45 * time.Second

	var gotSpec redisadapter.Spec
	var gotKey string
	var gotTTL time.Duration
	runner := newBehaviorPendingReconcileRunnerWithHooks(
		opts,
		projector,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		func(_ context.Context, spec redisadapter.Spec, key string, ttl time.Duration) (*redisadapter.Lease, bool, error) {
			gotSpec = spec
			gotKey = key
			gotTTL = ttl
			return &redisadapter.Lease{Key: "lock-key", Token: "token"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if gotSpec.Name != redisadapter.Specs.BehaviorPendingReconcile.Name {
		t.Fatalf("spec.name = %q, want %q", gotSpec.Name, redisadapter.Specs.BehaviorPendingReconcile.Name)
	}
	if gotKey != opts.LockKey {
		t.Fatalf("key = %q, want %q", gotKey, opts.LockKey)
	}
	if gotTTL != opts.LockTTL {
		t.Fatalf("ttl = %s, want %s", gotTTL, opts.LockTTL)
	}
}

func TestBehaviorPendingReconcileRunOnceSkipsWhenLockNotAcquired(t *testing.T) {
	projector := &fakeBehaviorProjector{}
	runner := newBehaviorPendingReconcileRunnerWithHooks(
		newTestBehaviorPendingReconcileOptions(),
		projector,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return nil, false, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if calls := projector.calls(); len(calls) != 0 {
		t.Fatalf("expected no reconcile calls when lock not acquired, got %v", calls)
	}
}

func TestBehaviorPendingReconcileRunOnceUsesBatchLimit(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	projector := &fakeBehaviorProjector{}
	opts := newTestBehaviorPendingReconcileOptions()
	opts.BatchLimit = 42

	runner := newBehaviorPendingReconcileRunnerWithHooks(
		opts,
		projector,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if calls := projector.calls(); len(calls) != 1 || calls[0] != 42 {
		t.Fatalf("unexpected reconcile calls: %v", calls)
	}
	if lock.releases() != 1 {
		t.Fatalf("expected lock release once, got %d", lock.releases())
	}
}

func TestBehaviorPendingReconcileRunOnceReturnsProjectorError(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	projector := &fakeBehaviorProjector{err: errors.New("reconcile failed")}
	runner := newBehaviorPendingReconcileRunnerWithHooks(
		newTestBehaviorPendingReconcileOptions(),
		projector,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err == nil {
		t.Fatalf("expected reconcile error")
	}
}

func TestBehaviorPendingReconcileMultiInstanceOnlyOneExecutes(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	block := make(chan struct{})
	started := make(chan int, 1)

	projector1 := &fakeBehaviorProjector{started: started, block: block}
	projector2 := &fakeBehaviorProjector{}
	opts := newTestBehaviorPendingReconcileOptions()

	runner1 := newBehaviorPendingReconcileRunnerWithHooks(
		opts,
		projector1,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		lock.acquire,
		lock.release,
	)
	runner2 := newBehaviorPendingReconcileRunnerWithHooks(
		opts,
		projector2,
		&redisadapter.Manager{},
		newTestBehaviorLockBuilder(),
		lock.acquire,
		lock.release,
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runner1.runOnce(context.Background())
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first reconcile runner did not start in time")
	}

	if err := runner2.runOnce(context.Background()); err != nil {
		t.Fatalf("second runner returned error: %v", err)
	}

	close(block)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("first runner returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("first runner did not complete in time")
	}

	if calls := projector1.calls(); len(calls) != 1 {
		t.Fatalf("unexpected first runner calls: %v", calls)
	}
	if calls := projector2.calls(); len(calls) != 0 {
		t.Fatalf("expected second runner to skip execution, got %v", calls)
	}
}

func newTestBehaviorPendingReconcileOptions() *apiserveroptions.BehaviorPendingReconcileOptions {
	return &apiserveroptions.BehaviorPendingReconcileOptions{
		Enable:     true,
		Interval:   10 * time.Second,
		BatchLimit: 100,
		LockKey:    "qs:behavior-pending-reconcile:test",
		LockTTL:    30 * time.Second,
	}
}

func newTestBehaviorLockBuilder() *keyspace.Builder {
	return keyspace.NewBuilderWithNamespace(
		keyspace.ComposeNamespace("apiserver-test", "cache:lock"),
	)
}

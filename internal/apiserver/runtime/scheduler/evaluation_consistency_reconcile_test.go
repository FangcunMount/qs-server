package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	evaluationScheduler "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/redisadapter"
)

type fakeEvaluationConsistencyService struct {
	mu      sync.Mutex
	limits  []int
	err     error
	started chan int
	block   <-chan struct{}
}

func (f *fakeEvaluationConsistencyService) AuditOnce(ctx context.Context, limit int) (int, error) {
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

func (f *fakeEvaluationConsistencyService) calls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()

	limits := make([]int, len(f.limits))
	copy(limits, f.limits)
	return limits
}

func TestNewEvaluationConsistencyReconcileRunner(t *testing.T) {
	opts := newTestEvaluationConsistencyReconcileOptions()
	service := &fakeEvaluationConsistencyService{}

	if runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		&apiserveroptions.EvaluationConsistencyReconcileOptions{Enable: false},
		service,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected disabled reconcile runner to return nil")
	}

	if runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		opts,
		nil,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil service to return nil")
	}

	if runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		opts,
		service,
		nil,
		newTestEvaluationConsistencyLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected nil lock manager to return nil")
	}

	invalid := newTestEvaluationConsistencyReconcileOptions()
	invalid.Interval = 0
	if runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		invalid,
		service,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	); runner != nil {
		t.Fatalf("expected invalid interval to return nil")
	}
}

func TestEvaluationConsistencyReconcileLockKeyUsesLockNamespace(t *testing.T) {
	runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		newTestEvaluationConsistencyReconcileOptions(),
		&fakeEvaluationConsistencyService{},
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "t"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	)
	if got := runner.lockKey(); got != "apiserver-test:cache:lock:qs:evaluation-consistency-reconcile:test" {
		t.Fatalf("unexpected lock key: %s", got)
	}
}

func TestEvaluationConsistencyReconcileRunOnceUsesConfiguredLockOverride(t *testing.T) {
	service := &fakeEvaluationConsistencyService{}
	opts := newTestEvaluationConsistencyReconcileOptions()
	opts.LockKey = "qs:evaluation-consistency-reconcile:custom"
	opts.LockTTL = 45 * time.Second

	var gotSpec redisadapter.Spec
	var gotKey string
	var gotTTL time.Duration
	runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		opts,
		service,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
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
	if gotSpec.Name != workloadSpec(locklease.WorkloadEvaluationConsistencyReconcile).Name {
		t.Fatalf("spec.name = %q, want %q", gotSpec.Name, workloadSpec(locklease.WorkloadEvaluationConsistencyReconcile).Name)
	}
	if gotKey != opts.LockKey {
		t.Fatalf("key = %q, want %q", gotKey, opts.LockKey)
	}
	if gotTTL != opts.LockTTL {
		t.Fatalf("ttl = %s, want %s", gotTTL, opts.LockTTL)
	}
}

func TestEvaluationConsistencyReconcileRunOnceSkipsWhenLockNotAcquired(t *testing.T) {
	service := &fakeEvaluationConsistencyService{}
	runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		newTestEvaluationConsistencyReconcileOptions(),
		service,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return nil, false, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error { return nil },
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if calls := service.calls(); len(calls) != 0 {
		t.Fatalf("expected no reconcile calls when lock not acquired, got %v", calls)
	}
}

func TestEvaluationConsistencyReconcileRunOnceUsesBatchLimit(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	service := &fakeEvaluationConsistencyService{}
	opts := newTestEvaluationConsistencyReconcileOptions()
	opts.BatchLimit = 42

	runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		opts,
		service,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce returned error: %v", err)
	}
	if calls := service.calls(); len(calls) != 1 || calls[0] != 42 {
		t.Fatalf("unexpected reconcile calls: %v", calls)
	}
	if lock.releases() != 1 {
		t.Fatalf("expected lock release once, got %d", lock.releases())
	}
}

func TestEvaluationConsistencyReconcileRunOnceReturnsServiceError(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	service := &fakeEvaluationConsistencyService{err: errors.New("reconcile failed")}
	runner := newEvaluationConsistencyReconcileRunnerWithHooks(
		newTestEvaluationConsistencyReconcileOptions(),
		service,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
		lock.acquire,
		lock.release,
	)

	if err := runner.runOnce(context.Background()); err == nil {
		t.Fatalf("expected reconcile error")
	}
}

func TestEvaluationConsistencyReconcileMultiInstanceOnlyOneExecutes(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	block := make(chan struct{})
	started := make(chan int, 1)

	service1 := &fakeEvaluationConsistencyService{started: started, block: block}
	service2 := &fakeEvaluationConsistencyService{}
	opts := newTestEvaluationConsistencyReconcileOptions()

	runner1 := newEvaluationConsistencyReconcileRunnerWithHooks(
		opts,
		service1,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
		lock.acquire,
		lock.release,
	)
	runner2 := newEvaluationConsistencyReconcileRunnerWithHooks(
		opts,
		service2,
		&redisadapter.Manager{},
		newTestEvaluationConsistencyLockBuilder(),
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

	if calls := service1.calls(); len(calls) != 1 {
		t.Fatalf("unexpected first runner calls: %v", calls)
	}
	if calls := service2.calls(); len(calls) != 0 {
		t.Fatalf("expected second runner to skip execution, got %v", calls)
	}
}

var _ evaluationScheduler.Service = (*fakeEvaluationConsistencyService)(nil)

func newTestEvaluationConsistencyReconcileOptions() *apiserveroptions.EvaluationConsistencyReconcileOptions {
	return &apiserveroptions.EvaluationConsistencyReconcileOptions{
		Enable:     true,
		Interval:   10 * time.Second,
		BatchLimit: 100,
		LockKey:    "qs:evaluation-consistency-reconcile:test",
		LockTTL:    30 * time.Second,
	}
}

func newTestEvaluationConsistencyLockBuilder() *keyspace.Builder {
	return keyspace.NewBuilderWithNamespace(
		keyspace.ComposeNamespace("apiserver-test", "cache:lock"),
	)
}

package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/redisadapter"
)

func TestLeaderLockRunExecutesBodyAndReleasesWhenAcquired(t *testing.T) {
	bodyCalls := 0
	releaseCalls := 0
	lock := newLeaderLock(
		workloadSpec(locklease.WorkloadPlanSchedulerLeader),
		"qs:plan-scheduler:test",
		30*time.Second,
		keyspace.NewBuilderWithNamespace("apiserver-test:cache:lock"),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "lock-key", Token: "token"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error {
			releaseCalls++
			return nil
		},
	)

	if err := lock.Run(context.Background(), leaderLockRunOptions{}, func(context.Context) error {
		bodyCalls++
		return nil
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bodyCalls != 1 {
		t.Fatalf("body calls = %d, want 1", bodyCalls)
	}
	if releaseCalls != 1 {
		t.Fatalf("release calls = %d, want 1", releaseCalls)
	}
}

func TestLeaderLockRunSkipsBodyWhenNotAcquired(t *testing.T) {
	bodyCalls := 0
	releaseCalls := 0
	var skippedKey string
	lock := newLeaderLock(
		workloadSpec(locklease.WorkloadStatisticsSyncLeader),
		"qs:statistics-sync:test",
		time.Minute,
		keyspace.NewBuilderWithNamespace("apiserver-test:cache:lock"),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return nil, false, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error {
			releaseCalls++
			return nil
		},
	)

	if err := lock.Run(context.Background(), leaderLockRunOptions{
		OnNotAcquired: func(lockKey string) {
			skippedKey = lockKey
		},
	}, func(context.Context) error {
		bodyCalls++
		return nil
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bodyCalls != 0 {
		t.Fatalf("body calls = %d, want 0", bodyCalls)
	}
	if releaseCalls != 0 {
		t.Fatalf("release calls = %d, want 0", releaseCalls)
	}
	if skippedKey != "apiserver-test:cache:lock:qs:statistics-sync:test" {
		t.Fatalf("skipped key = %q", skippedKey)
	}
}

func TestLeaderLockRunWrapsAcquireError(t *testing.T) {
	acquireErr := errors.New("redis unavailable")
	lock := newLeaderLock(
		workloadSpec(locklease.WorkloadEvaluationConsistencyReconcile),
		"qs:evaluation-consistency-reconcile:test",
		time.Minute,
		nil,
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return nil, false, acquireErr
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error {
			t.Fatal("release must not be called on acquire error")
			return nil
		},
	)

	err := lock.Run(context.Background(), leaderLockRunOptions{
		AcquireError: "failed to acquire behavior lock",
	}, func(context.Context) error {
		t.Fatal("body must not be called on acquire error")
		return nil
	})
	if err == nil {
		t.Fatal("expected acquire error")
	}
	if !errors.Is(err, acquireErr) {
		t.Fatalf("Run() error = %v, want wrapped acquire error", err)
	}
	if !strings.Contains(err.Error(), "failed to acquire behavior lock") {
		t.Fatalf("Run() error = %q, want configured prefix", err.Error())
	}
}

func TestLeaderLockRunPreservesBodyErrorAndReportsReleaseError(t *testing.T) {
	bodyErr := errors.New("body failed")
	releaseErr := errors.New("release failed")
	var gotReleaseKey string
	var gotReleaseErr error
	lock := newLeaderLock(
		workloadSpec(locklease.WorkloadPlanSchedulerLeader),
		"qs:plan-scheduler:test",
		time.Minute,
		keyspace.NewBuilderWithNamespace("apiserver-test:cache:lock"),
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "lock-key", Token: "token"}, true, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error {
			return releaseErr
		},
	)

	err := lock.Run(context.Background(), leaderLockRunOptions{
		OnReleaseError: func(lockKey string, err error) {
			gotReleaseKey = lockKey
			gotReleaseErr = err
		},
	}, func(context.Context) error {
		return bodyErr
	})
	if !errors.Is(err, bodyErr) {
		t.Fatalf("Run() error = %v, want body error", err)
	}
	if gotReleaseKey != "apiserver-test:cache:lock:qs:plan-scheduler:test" {
		t.Fatalf("release key = %q", gotReleaseKey)
	}
	if !errors.Is(gotReleaseErr, releaseErr) {
		t.Fatalf("release error = %v, want %v", gotReleaseErr, releaseErr)
	}
}

func TestLeaderLockDisplayKeyUsesDefaultBuilderWhenBuilderIsNil(t *testing.T) {
	lock := newLeaderLock(
		workloadSpec(locklease.WorkloadPlanSchedulerLeader),
		"qs:plan-scheduler:test",
		time.Minute,
		nil,
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			return nil, false, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error {
			return nil
		},
	)

	if got := lock.DisplayKey(); got != "qs:plan-scheduler:test" {
		t.Fatalf("DisplayKey() = %q, want raw key with default builder", got)
	}
}

func TestLeaderLockRunUsesRunnerContextAndReportsReleaseError(t *testing.T) {
	type contextKey struct{}
	releaseErr := errors.New("release failed")
	var gotReleaseErr error
	runner := leaderLockRunnerStub{run: func(
		ctx context.Context,
		workload locklease.WorkloadID,
		key string,
		ttl time.Duration,
		body func(context.Context) error,
	) (locklease.RunResult, error) {
		if workload != locklease.WorkloadPlanSchedulerLeader || key != "qs:plan-scheduler:runner" || ttl != time.Minute {
			t.Fatalf("runner input = %s, %q, %s", workload, key, ttl)
		}
		child := context.WithValue(ctx, contextKey{}, "runner-child")
		if err := body(child); err != nil {
			return locklease.RunResult{Acquired: true}, err
		}
		return locklease.RunResult{Acquired: true, ReleaseErr: releaseErr}, nil
	}}
	lock := newLeaderLock(
		workloadSpec(locklease.WorkloadPlanSchedulerLeader),
		"qs:plan-scheduler:runner",
		time.Minute,
		nil,
		func(context.Context, redisadapter.Spec, string, time.Duration) (*redisadapter.Lease, bool, error) {
			t.Fatal("legacy acquire must not be called when runner is available")
			return nil, false, nil
		},
		func(context.Context, redisadapter.Spec, string, *redisadapter.Lease) error {
			t.Fatal("legacy release must not be called when runner is available")
			return nil
		},
		runner,
	)

	err := lock.Run(context.Background(), leaderLockRunOptions{
		OnReleaseError: func(_ string, err error) { gotReleaseErr = err },
	}, func(ctx context.Context) error {
		if got := ctx.Value(contextKey{}); got != "runner-child" {
			t.Fatalf("body context value = %v, want runner child context", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !errors.Is(gotReleaseErr, releaseErr) {
		t.Fatalf("release error = %v, want %v", gotReleaseErr, releaseErr)
	}
}

func TestLeaderLockRunnerContentionSkipsBody(t *testing.T) {
	runner := leaderLockRunnerStub{run: func(context.Context, locklease.WorkloadID, string, time.Duration, func(context.Context) error) (locklease.RunResult, error) {
		return locklease.RunResult{}, nil
	}}
	lock := newLeaderLock(
		workloadSpec(locklease.WorkloadStatisticsSyncLeader),
		"qs:statistics-sync:runner",
		time.Minute,
		nil,
		nil,
		nil,
		runner,
	)
	skipped := false
	if err := lock.Run(context.Background(), leaderLockRunOptions{
		OnNotAcquired: func(string) { skipped = true },
	}, func(context.Context) error {
		t.Fatal("body must not run on contention")
		return nil
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !skipped {
		t.Fatal("contention callback was not called")
	}
}

type leaderLockRunnerStub struct {
	run func(context.Context, locklease.WorkloadID, string, time.Duration, func(context.Context) error) (locklease.RunResult, error)
}

func (r leaderLockRunnerStub) Run(ctx context.Context, workload locklease.WorkloadID, key string, ttl time.Duration, body func(context.Context) error) (locklease.RunResult, error) {
	return r.run(ctx, workload, key, ttl, body)
}

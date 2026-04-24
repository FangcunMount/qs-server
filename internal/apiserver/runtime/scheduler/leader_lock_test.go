package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
)

func TestLeaderLockRunExecutesBodyAndReleasesWhenAcquired(t *testing.T) {
	bodyCalls := 0
	releaseCalls := 0
	lock := newLeaderLock(
		redislock.Specs.PlanSchedulerLeader,
		"qs:plan-scheduler:test",
		30*time.Second,
		rediskey.NewBuilderWithNamespace("apiserver-test:cache:lock"),
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return &redislock.Lease{Key: "lock-key", Token: "token"}, true, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error {
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
		redislock.Specs.StatisticsSyncLeader,
		"qs:statistics-sync:test",
		time.Minute,
		rediskey.NewBuilderWithNamespace("apiserver-test:cache:lock"),
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return nil, false, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error {
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
		redislock.Specs.BehaviorPendingReconcile,
		"qs:behavior-pending-reconcile:test",
		time.Minute,
		nil,
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return nil, false, acquireErr
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error {
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
		redislock.Specs.PlanSchedulerLeader,
		"qs:plan-scheduler:test",
		time.Minute,
		rediskey.NewBuilderWithNamespace("apiserver-test:cache:lock"),
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return &redislock.Lease{Key: "lock-key", Token: "token"}, true, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error {
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
		redislock.Specs.PlanSchedulerLeader,
		"qs:plan-scheduler:test",
		time.Minute,
		nil,
		func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error) {
			return nil, false, nil
		},
		func(context.Context, redislock.Spec, string, *redislock.Lease) error {
			return nil
		},
	)

	if got := lock.DisplayKey(); got != "qs:plan-scheduler:test" {
		t.Fatalf("DisplayKey() = %q, want raw key with default builder", got)
	}
}

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
)

type leaderLockAcquireFunc func(context.Context, locklease.Spec, string, time.Duration) (*locklease.Lease, bool, error)

type leaderLockReleaseFunc func(context.Context, locklease.Spec, string, *locklease.Lease) error

type leaderLeaseRunner interface {
	DisplayKey() string
	Run(ctx context.Context, opts leaderLockRunOptions, body func(context.Context) error) error
}

type leaderLock struct {
	workload   locklease.WorkloadID
	spec       locklease.Spec
	rawKey     string
	ttl        time.Duration
	displayKey string
	acquire    leaderLockAcquireFunc
	release    leaderLockReleaseFunc
	runner     locklease.Runner
}

type leaderLockRunOptions struct {
	AcquireError   string
	OnNotAcquired  func(lockKey string)
	OnReleaseError func(lockKey string, err error)
}

func workloadSpec(id locklease.WorkloadID) locklease.Spec {
	capability, _ := locklease.Lookup(id)
	return capability.Spec
}

func leaseRunner(manager locklease.Manager) locklease.Runner {
	runner, _ := manager.(locklease.Runner)
	return runner
}

func newLeaderLock(
	spec locklease.Spec,
	rawKey string,
	ttl time.Duration,
	builder *keyspace.Builder,
	acquire leaderLockAcquireFunc,
	release leaderLockReleaseFunc,
	runners ...locklease.Runner,
) *leaderLock {
	if builder == nil {
		builder = keyspace.NewBuilder()
	}
	lock := &leaderLock{
		spec:       spec,
		rawKey:     rawKey,
		ttl:        ttl,
		displayKey: builder.BuildLockKey(rawKey),
		acquire:    acquire,
		release:    release,
	}
	for _, runner := range runners {
		if runner != nil {
			lock.runner = runner
			break
		}
	}
	for _, capability := range locklease.All() {
		if capability.Spec.Name == spec.Name {
			lock.workload = capability.ID
			break
		}
	}
	return lock
}

func (l *leaderLock) DisplayKey() string {
	if l == nil {
		return ""
	}
	return l.displayKey
}

func (l *leaderLock) Run(ctx context.Context, opts leaderLockRunOptions, body func(context.Context) error) error {
	if l != nil && l.runner != nil && l.workload != "" {
		result, err := l.runner.Run(ctx, l.workload, l.rawKey, l.ttl, body)
		if err != nil {
			if errors.Is(err, locklease.ErrLeaseAcquireFailed) && opts.AcquireError != "" {
				return fmt.Errorf("%s: %w", opts.AcquireError, err)
			}
			return err
		}
		if !result.Acquired {
			if opts.OnNotAcquired != nil {
				opts.OnNotAcquired(l.displayKey)
			}
			return nil
		}
		if result.ReleaseErr != nil && opts.OnReleaseError != nil {
			opts.OnReleaseError(l.displayKey, result.ReleaseErr)
		}
		return nil
	}
	if l == nil || l.acquire == nil || l.release == nil {
		return fmt.Errorf("leader lock is unavailable")
	}

	lease, acquired, err := l.acquire(ctx, l.spec, l.rawKey, l.ttl)
	if err != nil {
		if opts.AcquireError == "" {
			return err
		}
		return fmt.Errorf("%s: %w", opts.AcquireError, err)
	}
	if !acquired {
		if opts.OnNotAcquired != nil {
			opts.OnNotAcquired(l.displayKey)
		}
		return nil
	}

	defer func() {
		if err := l.release(context.Background(), l.spec, l.rawKey, lease); err != nil && opts.OnReleaseError != nil {
			opts.OnReleaseError(l.displayKey, err)
		}
	}()

	if body == nil {
		return nil
	}
	return body(ctx)
}

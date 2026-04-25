package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
)

type leaderLockAcquireFunc func(context.Context, redislock.Spec, string, time.Duration) (*redislock.Lease, bool, error)

type leaderLockReleaseFunc func(context.Context, redislock.Spec, string, *redislock.Lease) error

type leaderLeaseRunner interface {
	DisplayKey() string
	Run(ctx context.Context, opts leaderLockRunOptions, body func(context.Context) error) error
}

type leaderLock struct {
	spec       redislock.Spec
	rawKey     string
	ttl        time.Duration
	displayKey string
	acquire    leaderLockAcquireFunc
	release    leaderLockReleaseFunc
}

type leaderLockRunOptions struct {
	AcquireError   string
	OnNotAcquired  func(lockKey string)
	OnReleaseError func(lockKey string, err error)
}

func newLeaderLock(
	spec redislock.Spec,
	rawKey string,
	ttl time.Duration,
	builder *rediskey.Builder,
	acquire leaderLockAcquireFunc,
	release leaderLockReleaseFunc,
) *leaderLock {
	if builder == nil {
		builder = rediskey.NewBuilder()
	}
	return &leaderLock{
		spec:       spec,
		rawKey:     rawKey,
		ttl:        ttl,
		displayKey: builder.BuildLockKey(rawKey),
		acquire:    acquire,
		release:    release,
	}
}

func (l *leaderLock) DisplayKey() string {
	if l == nil {
		return ""
	}
	return l.displayKey
}

func (l *leaderLock) Run(ctx context.Context, opts leaderLockRunOptions, body func(context.Context) error) error {
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

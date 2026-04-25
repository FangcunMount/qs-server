package redisops

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultSubmitInflightTTL = 5 * time.Minute
	defaultSubmitResultTTL   = 30 * time.Minute
)

type SubmitGuard struct {
	opsHandle *redisplane.Handle
	lockMgr   *redislock.Manager
}

func NewSubmitGuard(opsHandle *redisplane.Handle, lockMgr *redislock.Manager) *SubmitGuard {
	return &SubmitGuard{
		opsHandle: opsHandle,
		lockMgr:   lockMgr,
	}
}

func (g *SubmitGuard) Begin(ctx context.Context, key string) (string, *redislock.Lease, bool, error) {
	if g == nil || key == "" {
		return "", nil, true, nil
	}
	if doneID, ok, err := g.lookupDone(ctx, key); err != nil {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockError)
		return "", nil, false, err
	} else if ok {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeIdempotencyHit)
		return doneID, nil, false, nil
	}
	if g.lockMgr == nil {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeDegradedOpen)
		return "", nil, true, nil
	}
	lease, acquired, err := g.lockMgr.AcquireSpec(ctx, redislock.Specs.CollectionSubmit, submitInflightKey(key), defaultSubmitInflightTTL)
	if err != nil {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockError)
		return "", nil, false, err
	}
	if acquired {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockAcquired)
	} else {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockContention)
	}
	return "", lease, acquired, nil
}

func (g *SubmitGuard) Complete(ctx context.Context, key string, lease *redislock.Lease, answerSheetID string) error {
	if g == nil {
		return nil
	}
	if answerSheetID != "" && g.opsHandle != nil && g.opsHandle.Client != nil && g.opsHandle.Builder != nil {
		if err := g.opsHandle.Client.Set(ctx, g.opsHandle.Builder.BuildLockKey(submitDoneKey(key)), answerSheetID, defaultSubmitResultTTL).Err(); err != nil {
			g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockError)
			return err
		}
	}
	if g.lockMgr != nil {
		return g.lockMgr.ReleaseSpec(context.Background(), redislock.Specs.CollectionSubmit, submitInflightKey(key), lease)
	}
	return nil
}

func (g *SubmitGuard) Abort(ctx context.Context, key string, lease *redislock.Lease) error {
	if g == nil || g.lockMgr == nil {
		return nil
	}
	return g.lockMgr.ReleaseSpec(ctx, redislock.Specs.CollectionSubmit, submitInflightKey(key), lease)
}

func (g *SubmitGuard) lookupDone(ctx context.Context, key string) (string, bool, error) {
	if g == nil || g.opsHandle == nil || g.opsHandle.Client == nil || g.opsHandle.Builder == nil {
		return "", false, nil
	}
	value, err := g.opsHandle.Client.Get(ctx, g.opsHandle.Builder.BuildLockKey(submitDoneKey(key))).Result()
	if err != nil {
		if err == redis.Nil {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

func submitInflightKey(key string) string {
	return "submit:idempotency:" + key + ":lock"
}

func submitDoneKey(key string) string {
	return "submit:idempotency:" + key + ":done"
}

func (g *SubmitGuard) observe(ctx context.Context, kind resilienceplane.ProtectionKind, outcome resilienceplane.Outcome) {
	resilienceplane.Observe(ctx, resilienceplane.DefaultObserver(), kind, resilienceplane.Subject{
		Component: "collection-server",
		Scope:     "answersheet_submit",
		Resource:  "submit_guard",
		Strategy:  "redis_lock",
	}, outcome)
}

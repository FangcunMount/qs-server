package redisops

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	redis "github.com/redis/go-redis/v9"
)

const (
	defaultSubmitInflightTTL = 5 * time.Minute
	defaultSubmitResultTTL   = 30 * time.Minute
)

type SubmitGuard struct {
	opsHandle *redisruntime.Handle
	lockMgr   locklease.Manager
	runner    locklease.Runner
	observer  resilienceplane.Observer
}

// NewSubmitGuardWithRunner creates the closure-based guard used by production composition.
func NewSubmitGuardWithRunner(opsHandle *redisruntime.Handle, runner locklease.Runner) *SubmitGuard {
	return &SubmitGuard{
		opsHandle: opsHandle,
		runner:    runner,
		observer:  defaultObserver(nil),
	}
}

func NewSubmitGuard(opsHandle *redisruntime.Handle, lockMgr locklease.Manager) *SubmitGuard {
	return NewSubmitGuardWithObserver(opsHandle, lockMgr, nil)
}

func NewSubmitGuardWithObserver(opsHandle *redisruntime.Handle, lockMgr locklease.Manager, observer resilienceplane.Observer) *SubmitGuard {
	return &SubmitGuard{
		opsHandle: opsHandle,
		lockMgr:   lockMgr,
		observer:  defaultObserver(observer),
	}
}

func (g *SubmitGuard) Begin(ctx context.Context, key string) (string, *locklease.Lease, bool, error) {
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
	capability, _ := locklease.Lookup(locklease.WorkloadCollectionSubmit)
	lease, acquired, err := g.lockMgr.AcquireSpec(ctx, capability.Spec, submitInflightKey(key), defaultSubmitInflightTTL)
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

func (g *SubmitGuard) Complete(ctx context.Context, key string, lease *locklease.Lease, answerSheetID string) error {
	if g == nil {
		return nil
	}
	if answerSheetID != "" && g.opsHandle != nil && g.opsHandle.Client != nil {
		if err := g.opsHandle.Client.Set(ctx, g.opsKeyspace().IdempotencyDone(key), answerSheetID, defaultSubmitResultTTL).Err(); err != nil {
			g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockError)
			return err
		}
	}
	if g.lockMgr != nil {
		capability, _ := locklease.Lookup(locklease.WorkloadCollectionSubmit)
		return g.lockMgr.ReleaseSpec(context.Background(), capability.Spec, submitInflightKey(key), lease)
	}
	return nil
}

func (g *SubmitGuard) Abort(ctx context.Context, key string, lease *locklease.Lease) error {
	if g == nil || g.lockMgr == nil {
		return nil
	}
	capability, _ := locklease.Lookup(locklease.WorkloadCollectionSubmit)
	return g.lockMgr.ReleaseSpec(ctx, capability.Spec, submitInflightKey(key), lease)
}

// Run executes a submit closure while owning the lease. The done marker is
// persisted inside the closure and therefore before Runner releases the lock.
func (g *SubmitGuard) Run(ctx context.Context, key string, body func(context.Context) (string, error)) (string, bool, error) {
	if g == nil || key == "" {
		if body == nil {
			return "", true, nil
		}
		value, err := body(ctx)
		return value, true, err
	}
	if doneID, ok, err := g.lookupDone(ctx, key); err != nil {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockError)
		return "", false, err
	} else if ok {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeIdempotencyHit)
		return doneID, false, nil
	}
	if g.runner == nil {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeDegradedOpen)
		if body == nil {
			return "", true, nil
		}
		value, err := body(ctx)
		return value, true, err
	}

	var answerSheetID string
	result, err := g.runner.Run(ctx, locklease.WorkloadCollectionSubmit, submitInflightKey(key), defaultSubmitInflightTTL, func(runCtx context.Context) error {
		if body == nil {
			return nil
		}
		value, bodyErr := body(runCtx)
		if bodyErr != nil {
			return bodyErr
		}
		answerSheetID = value
		if value != "" && g.opsHandle != nil && g.opsHandle.Client != nil {
			if setErr := g.opsHandle.Client.Set(runCtx, g.opsKeyspace().IdempotencyDone(key), value, defaultSubmitResultTTL).Err(); setErr != nil {
				g.observe(runCtx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockError)
				return setErr
			}
		}
		return nil
	})
	if err != nil {
		return "", result.Acquired, err
	}
	if !result.Acquired {
		g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockContention)
		return "", false, nil
	}
	g.observe(ctx, resilienceplane.ProtectionIdempotency, resilienceplane.OutcomeLockAcquired)
	return answerSheetID, true, nil
}

func (g *SubmitGuard) lookupDone(ctx context.Context, key string) (string, bool, error) {
	if g == nil || g.opsHandle == nil || g.opsHandle.Client == nil {
		return "", false, nil
	}
	value, err := g.opsHandle.Client.Get(ctx, g.opsKeyspace().IdempotencyDone(key)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

func (g *SubmitGuard) opsKeyspace() opsKeyspace {
	if g == nil || g.opsHandle == nil {
		return newOpsKeyspace("")
	}
	return newOpsKeyspace(g.opsHandle.Namespace)
}

func submitInflightKey(key string) string {
	return "submit:idempotency:" + key + ":lock"
}

func submitDoneKey(key string) string {
	return "submit:idempotency:" + key + ":done"
}

func (g *SubmitGuard) observe(ctx context.Context, kind resilienceplane.ProtectionKind, outcome resilienceplane.Outcome) {
	observer := resilienceplane.DefaultObserver()
	if g != nil && g.observer != nil {
		observer = g.observer
	}
	resilienceplane.Observe(ctx, observer, kind, resilienceplane.Subject{
		Component: "collection-server",
		Scope:     "answersheet_submit",
		Resource:  "submit_guard",
		Strategy:  "redis_lock",
	}, outcome)
}

func defaultObserver(observer resilienceplane.Observer) resilienceplane.Observer {
	if observer != nil {
		return observer
	}
	return resilienceplane.DefaultObserver()
}

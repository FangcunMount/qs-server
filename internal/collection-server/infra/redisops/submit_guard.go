package redisops

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

const (
	defaultSubmitInflightTTL = 5 * time.Minute
)

type SubmitGuard struct {
	lockMgr  locklease.Manager
	runner   locklease.Runner
	observer resilience.Observer
}

// NewSubmitGuardWithRunner creates the closure-based guard used by production composition.
func NewSubmitGuardWithRunner(_ *redisruntime.Handle, runner locklease.Runner) *SubmitGuard {
	return &SubmitGuard{
		runner:   runner,
		observer: defaultObserver(nil),
	}
}

func NewSubmitGuard(opsHandle *redisruntime.Handle, lockMgr locklease.Manager) *SubmitGuard {
	return NewSubmitGuardWithObserver(opsHandle, lockMgr, nil)
}

func NewSubmitGuardWithObserver(_ *redisruntime.Handle, lockMgr locklease.Manager, observer resilience.Observer) *SubmitGuard {
	return &SubmitGuard{
		lockMgr:  lockMgr,
		observer: defaultObserver(observer),
	}
}

func (g *SubmitGuard) Begin(ctx context.Context, key string) (string, *locklease.Lease, bool, error) {
	if g == nil || key == "" {
		return "", nil, true, nil
	}
	if g.lockMgr == nil {
		g.observe(ctx, resilience.ProtectionIdempotency, resilience.OutcomeDegradedOpen)
		return "", nil, true, nil
	}
	capability, _ := locklease.Lookup(locklease.WorkloadCollectionSubmit)
	lease, acquired, err := g.lockMgr.AcquireSpec(ctx, capability.Spec, submitInflightKey(key), defaultSubmitInflightTTL)
	if err != nil {
		g.observe(ctx, resilience.ProtectionIdempotency, resilience.OutcomeLockError)
		return "", nil, false, err
	}
	if acquired {
		g.observe(ctx, resilience.ProtectionIdempotency, resilience.OutcomeLockAcquired)
	} else {
		g.observe(ctx, resilience.ProtectionIdempotency, resilience.OutcomeLockContention)
	}
	return "", lease, acquired, nil
}

func (g *SubmitGuard) Complete(ctx context.Context, key string, lease *locklease.Lease, answerSheetID string) error {
	if g == nil {
		return nil
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

// Run executes a submit closure while owning an advisory cross-instance lease.
// Durable idempotency and final results live in Mongo, never in Redis. Lease
// infrastructure failures degrade open so the database constraint converges.
func (g *SubmitGuard) Run(ctx context.Context, key string, body func(context.Context) (string, error)) (string, bool, error) {
	if g == nil || key == "" {
		if body == nil {
			return "", true, nil
		}
		value, err := body(ctx)
		return value, true, err
	}
	if g.runner == nil {
		g.observe(ctx, resilience.ProtectionIdempotency, resilience.OutcomeDegradedOpen)
		if body == nil {
			return "", true, nil
		}
		value, err := body(ctx)
		return value, true, err
	}

	var answerSheetID string
	var bodyCalled bool
	var bodyErr error
	result, err := g.runner.Run(ctx, locklease.WorkloadCollectionSubmit, submitInflightKey(key), defaultSubmitInflightTTL, func(runCtx context.Context) error {
		bodyCalled = true
		if body == nil {
			return nil
		}
		value, err := body(runCtx)
		bodyErr = err
		if bodyErr != nil {
			return bodyErr
		}
		answerSheetID = value
		return nil
	})
	if err != nil {
		if bodyCalled {
			return answerSheetID, true, bodyErr
		}
		g.observe(ctx, resilience.ProtectionIdempotency, resilience.OutcomeDegradedOpen)
		if body == nil {
			return "", true, nil
		}
		value, fallbackErr := body(ctx)
		return value, true, fallbackErr
	}
	if !result.Acquired {
		g.observe(ctx, resilience.ProtectionIdempotency, resilience.OutcomeLockContention)
		return "", false, nil
	}
	g.observe(ctx, resilience.ProtectionIdempotency, resilience.OutcomeLockAcquired)
	return answerSheetID, true, nil
}

func submitInflightKey(key string) string {
	return "submit:idempotency:" + key + ":lock"
}

func (g *SubmitGuard) observe(ctx context.Context, kind resilience.ProtectionKind, outcome resilience.Outcome) {
	observer := resilience.DefaultObserver()
	if g != nil && g.observer != nil {
		observer = g.observer
	}
	resilience.Observe(ctx, observer, kind, resilience.Subject{
		Component: "collection-server",
		Scope:     "answersheet_submit",
		Resource:  "submit_guard",
		Strategy:  "redis_lock",
	}, outcome)
}

func defaultObserver(observer resilience.Observer) resilience.Observer {
	if observer != nil {
		return observer
	}
	return resilience.DefaultObserver()
}

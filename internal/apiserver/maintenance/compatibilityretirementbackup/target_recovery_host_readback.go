package compatibilityretirementbackup

import (
	"context"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

// These perform a fresh real readback through the same original borrowed handles
// and Window retained by the native recovery producer. A summary, zero pointer,
// copied proof or another host's handles cannot enable a rollback API launch.
// No connection/transaction/Window is opened, closed, reset or reconstructed.
func (v *TargetRecoveryVerification) VerifyHostRecoveredTargets(ctx context.Context, b TargetRecoveryBorrowed, r TargetRecoveryRequest, w *fence.MaintenanceWindow) error {
	if v == nil || v.self != v {
		return ErrRecoveryBinding
	}
	return verifyHostRecoveredTargetPlan(ctx, v.plan, b, r, w)
}

func (v *TargetLifecycleResult) VerifyHostRecoveredTargets(ctx context.Context, b TargetRecoveryBorrowed, r TargetRecoveryRequest, w *fence.MaintenanceWindow) error {
	if v == nil || v.self != v {
		return ErrRecoveryBinding
	}
	return verifyHostRecoveredTargetPlan(ctx, v.plan, b, r, w)
}

func verifyHostRecoveredTargetPlan(ctx context.Context, p *TargetRecoveryPlan, b TargetRecoveryBorrowed, r TargetRecoveryRequest, w *fence.MaintenanceWindow) error {
	if ctx == nil || ctx.Err() != nil || p == nil || p.self != p || w == nil || p.window != w || b.SQL == nil || b.Mongo == nil ||
		p.borrowed.SQL != b.SQL || p.borrowed.Mongo != b.Mongo {
		return ErrRecoveryBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.request != r || p.blocked {
		return ErrRecoveryBinding
	}
	q, c, e := w.RecoveryContext(ctx)
	if e != nil || !targetBoundContextValid(q, c) {
		if c != nil {
			c()
		}
		return ErrBudget
	}
	defer c()
	if e = p.checkBases(q, true); e != nil {
		return e
	}
	if p.archive == nil || p.archive.verifyAssets(q) != nil {
		return ErrSource
	}
	for i, o := range p.observations {
		if o.State != "existing_exact" && o.State != "restored_exact" {
			return ErrRecoveryState
		}
		present, e := p.checkTarget(q, i)
		if e != nil {
			return e
		}
		if !present {
			return ErrContent
		}
	}
	_, e = targetWindowMatches(q, w, p.request, "")
	return e
}

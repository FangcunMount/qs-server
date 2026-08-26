package scheduler

import (
	"context"
	"testing"
	"time"

	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

func TestAuthzRoleProjectionRunnerUsesLeaseAndBoundedBatch(t *testing.T) {
	t.Parallel()

	reconciler := &roleProjectionReconcilerStub{}
	locks := &roleProjectionLockRunnerStub{}
	runner := NewAuthzRoleProjectionRunner(reconciler, locks)

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}
	if locks.workload != locklease.WorkloadAuthzRoleProjectionReconcile || locks.key != roleProjectionReconcileLeaseKey {
		t.Fatalf("lease workload/key = %q/%q", locks.workload, locks.key)
	}
	if reconciler.limit != roleProjectionReconcileBatch {
		t.Fatalf("reconcile limit = %d, want %d", reconciler.limit, roleProjectionReconcileBatch)
	}
}

type roleProjectionReconcilerStub struct{ limit int }

func (s *roleProjectionReconcilerStub) ReconcilePending(_ context.Context, limit int) (operatorapp.RoleProjectionReconcileResult, error) {
	s.limit = limit
	return operatorapp.RoleProjectionReconcileResult{Scanned: 1, Succeeded: 1}, nil
}

type roleProjectionLockRunnerStub struct {
	workload locklease.WorkloadID
	key      string
}

func (s *roleProjectionLockRunnerStub) Run(ctx context.Context, workload locklease.WorkloadID, key string, _ time.Duration, body func(context.Context) error) (locklease.RunResult, error) {
	s.workload = workload
	s.key = key
	return locklease.RunResult{Acquired: true}, body(ctx)
}

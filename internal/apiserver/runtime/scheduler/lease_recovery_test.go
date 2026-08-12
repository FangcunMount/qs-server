package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	evaluationscheduler "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type leaseRecoveryStub struct {
	calls int
	err   error
}

func (s *leaseRecoveryStub) RecoverExpiredLeases(context.Context, time.Time, int) (int, error) {
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	return 2, nil
}

func TestLeaseRecoveryRunnersAreFailureIsolated(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	evaluation := &leaseRecoveryStub{err: errors.New("evaluation recovery failed")}
	interpretation := &leaseRecoveryStub{}
	evaluationRunner := newTestLeaseRecoveryRunner("evaluation_lease_recovery", locklease.WorkloadEvaluationLeaseRecovery, evaluation, lock)
	interpretationRunner := newTestLeaseRecoveryRunner("interpretation_lease_recovery", locklease.WorkloadInterpretationLeaseRecovery, interpretation, lock)

	if err := evaluationRunner.runOnce(context.Background()); err == nil {
		t.Fatal("expected evaluation recovery error")
	}
	if err := interpretationRunner.runOnce(context.Background()); err != nil {
		t.Fatalf("interpretation recovery was coupled to evaluation failure: %v", err)
	}
	if evaluation.calls != 1 || interpretation.calls != 1 {
		t.Fatalf("recovery calls = evaluation:%d interpretation:%d, want 1 each", evaluation.calls, interpretation.calls)
	}
}

func TestLeaseRecoveryRunnerUsesItsOwnBatchLimit(t *testing.T) {
	lock := &fakeSchedulerLockManager{}
	recoverer := &capturingLeaseRecoveryStub{}
	runner := newTestLeaseRecoveryRunner("evaluation_lease_recovery", locklease.WorkloadEvaluationLeaseRecovery, recoverer, lock)
	runner.opts.BatchLimit = 42
	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recoverer.limit != 42 {
		t.Fatalf("batch limit = %d, want 42", recoverer.limit)
	}
}

type capturingLeaseRecoveryStub struct{ limit int }

func (s *capturingLeaseRecoveryStub) RecoverExpiredLeases(_ context.Context, _ time.Time, limit int) (int, error) {
	s.limit = limit
	return 0, nil
}

func newTestLeaseRecoveryRunner(name string, workload locklease.WorkloadID, recoverer evaluationscheduler.LeaseRecoverer, lock *fakeSchedulerLockManager) *LeaseRecoveryRunner {
	opts := &apiserveroptions.LeaseRecoveryOptions{
		Enable: true, Interval: 10 * time.Second, BatchLimit: 100,
		LockKey: "qs:" + name + ":test", LockTTL: 30 * time.Second,
	}
	return &LeaseRecoveryRunner{
		name: name, opts: opts, recoverer: recoverer, now: time.Now,
		leader: newLeaderLock(
			workloadSpec(workload), opts.LockKey, opts.LockTTL,
			keyspace.NewBuilderWithNamespace(keyspace.ComposeNamespace("apiserver-test", "cache:lock")),
			lock.acquire, lock.release,
		),
	}
}

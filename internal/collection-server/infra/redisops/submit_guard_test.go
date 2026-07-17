package redisops

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestSubmitGuardSuppressesDuplicateAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	opsClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lockClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = opsClient.Close()
		_ = lockClient.Close()
	})

	opsHandle := &redisruntime.Handle{
		Family:  redisruntime.FamilyOps,
		Client:  opsClient,
		Builder: keyspace.NewBuilderWithNamespace("ops:runtime"),
	}
	lockHandle := &redisruntime.Handle{
		Family:  redisruntime.FamilyLock,
		Client:  lockClient,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
	}

	instanceA := NewSubmitGuard(opsHandle, redisadapter.NewManager("collection-server-a", "lock_lease", lockHandle))
	instanceB := NewSubmitGuard(opsHandle, redisadapter.NewManager("collection-server-b", "lock_lease", lockHandle))

	doneID, lease, acquired, err := instanceA.Begin(context.Background(), "req-1")
	if err != nil {
		t.Fatalf("instance A begin failed: %v", err)
	}
	if doneID != "" || !acquired || lease == nil {
		t.Fatalf("unexpected first begin result: doneID=%q acquired=%v lease=%+v", doneID, acquired, lease)
	}

	doneID, leaseB, acquired, err := instanceB.Begin(context.Background(), "req-1")
	if err != nil {
		t.Fatalf("instance B begin failed: %v", err)
	}
	if doneID != "" || acquired || leaseB != nil {
		t.Fatalf("expected contention on second begin, got doneID=%q acquired=%v lease=%+v", doneID, acquired, leaseB)
	}

	if err := instanceA.Complete(context.Background(), "req-1", lease, "answersheet-1"); err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	doneID, leaseB, acquired, err = instanceB.Begin(context.Background(), "req-1")
	if err != nil {
		t.Fatalf("instance B duplicate begin failed: %v", err)
	}
	if doneID != "answersheet-1" || acquired || leaseB != nil {
		t.Fatalf("expected duplicate suppression, got doneID=%q acquired=%v lease=%+v", doneID, acquired, leaseB)
	}
}

func TestSubmitGuardRunWritesDoneBeforeRunnerReleasesLease(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	opsHandle := &redisruntime.Handle{Family: redisruntime.FamilyOps, Client: client, Builder: keyspace.NewBuilderWithNamespace("ops:runtime")}
	lockHandle := &redisruntime.Handle{Family: redisruntime.FamilyLock, Client: client, Builder: keyspace.NewBuilderWithNamespace("cache:lock"), Configured: true, Available: true}
	locks := locksubsystem.New(locksubsystem.Options{Component: "collection-server", Handle: lockHandle})
	guard := NewSubmitGuardWithRunner(opsHandle, locks)
	lockKey := lockHandle.Builder.BuildLockKey(submitInflightKey("req-run"))

	doneID, acquired, err := guard.Run(context.Background(), "req-run", func(context.Context) (string, error) {
		if !mr.Exists(lockKey) {
			t.Fatal("lease must remain held while submit closure runs")
		}
		return "answersheet-1", nil
	})
	if err != nil || !acquired || doneID != "answersheet-1" {
		t.Fatalf("Run() = doneID=%q acquired=%v err=%v", doneID, acquired, err)
	}
	if mr.Exists(lockKey) {
		t.Fatal("lease was not released after done marker write")
	}
	if value, err := client.Get(context.Background(), guard.opsKeyspace().IdempotencyDone("req-run")).Result(); err != nil || value != "answersheet-1" {
		t.Fatalf("done marker = %q, %v", value, err)
	}

	doneID, acquired, err = guard.Run(context.Background(), "req-run", func(context.Context) (string, error) {
		t.Fatal("duplicate hit must not execute body")
		return "", nil
	})
	if err != nil || acquired || doneID != "answersheet-1" {
		t.Fatalf("duplicate Run() = doneID=%q acquired=%v err=%v", doneID, acquired, err)
	}
}

func TestSubmitGuardReleasesInFlightLeaseOnAbort(t *testing.T) {
	mr := miniredis.RunT(t)
	opsClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lockClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = opsClient.Close()
		_ = lockClient.Close()
	})

	guard := NewSubmitGuard(
		&redisruntime.Handle{
			Family:  redisruntime.FamilyOps,
			Client:  opsClient,
			Builder: keyspace.NewBuilderWithNamespace("ops:runtime"),
		},
		redisadapter.NewManager("collection-server", "lock_lease", &redisruntime.Handle{
			Family:  redisruntime.FamilyLock,
			Client:  lockClient,
			Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
		}),
	)

	_, lease, acquired, err := guard.Begin(context.Background(), "req-2")
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}
	if !acquired || lease == nil {
		t.Fatalf("expected acquire success, got acquired=%v lease=%+v", acquired, lease)
	}

	if err := guard.Abort(context.Background(), "req-2", lease); err != nil {
		t.Fatalf("abort failed: %v", err)
	}

	doneID, lease2, acquired, err := guard.Begin(context.Background(), "req-2")
	if err != nil {
		t.Fatalf("begin after abort failed: %v", err)
	}
	if doneID != "" || !acquired || lease2 == nil {
		t.Fatalf("expected lease reacquisition after abort, got doneID=%q acquired=%v lease=%+v", doneID, acquired, lease2)
	}
}

func TestSubmitGuardCompleteKeepsInFlightLeaseWhenDoneMarkerWriteFails(t *testing.T) {
	mr := miniredis.RunT(t)
	opsClientA := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	opsClientB := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lockClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = opsClientA.Close()
		_ = opsClientB.Close()
		_ = lockClient.Close()
	})

	lockHandle := &redisruntime.Handle{
		Family:  redisruntime.FamilyLock,
		Client:  lockClient,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
	}
	instanceA := NewSubmitGuard(
		&redisruntime.Handle{
			Family:  redisruntime.FamilyOps,
			Client:  opsClientA,
			Builder: keyspace.NewBuilderWithNamespace("ops:runtime"),
		},
		redisadapter.NewManager("collection-server-a", "lock_lease", lockHandle),
	)
	instanceB := NewSubmitGuard(
		&redisruntime.Handle{
			Family:  redisruntime.FamilyOps,
			Client:  opsClientB,
			Builder: keyspace.NewBuilderWithNamespace("ops:runtime"),
		},
		redisadapter.NewManager("collection-server-b", "lock_lease", lockHandle),
	)

	_, lease, acquired, err := instanceA.Begin(context.Background(), "req-3")
	if err != nil {
		t.Fatalf("instance A begin failed: %v", err)
	}
	if !acquired || lease == nil {
		t.Fatalf("expected instance A to acquire lock, got acquired=%v lease=%+v", acquired, lease)
	}

	if err := opsClientA.Close(); err != nil {
		t.Fatalf("close ops client A: %v", err)
	}
	if err := instanceA.Complete(context.Background(), "req-3", lease, "answersheet-3"); err == nil {
		t.Fatal("expected complete to fail when done marker write fails")
	}

	doneID, leaseB, acquired, err := instanceB.Begin(context.Background(), "req-3")
	if err != nil {
		t.Fatalf("instance B begin failed: %v", err)
	}
	if doneID != "" || acquired || leaseB != nil {
		t.Fatalf("expected in-flight lease to remain held, got doneID=%q acquired=%v lease=%+v", doneID, acquired, leaseB)
	}

	mr.FastForward(defaultSubmitInflightTTL + time.Second)

	doneID, leaseB, acquired, err = instanceB.Begin(context.Background(), "req-3")
	if err != nil {
		t.Fatalf("instance B begin after ttl failed: %v", err)
	}
	if doneID != "" || !acquired || leaseB == nil {
		t.Fatalf("expected lease acquisition after ttl expiry, got doneID=%q acquired=%v lease=%+v", doneID, acquired, leaseB)
	}
}

func TestSubmitGuardAllowsWhenDisabledOrKeyEmpty(t *testing.T) {
	ctx := context.Background()

	var nilGuard *SubmitGuard
	if doneID, lease, acquired, err := nilGuard.Begin(ctx, "req-disabled"); err != nil {
		t.Fatalf("nil guard Begin() error = %v", err)
	} else if doneID != "" || lease != nil || !acquired {
		t.Fatalf("nil guard Begin() = doneID=%q lease=%+v acquired=%v, want allow", doneID, lease, acquired)
	}
	if err := nilGuard.Complete(ctx, "req-disabled", nil, "answersheet-disabled"); err != nil {
		t.Fatalf("nil guard Complete() error = %v", err)
	}
	if err := nilGuard.Abort(ctx, "req-disabled", nil); err != nil {
		t.Fatalf("nil guard Abort() error = %v", err)
	}

	disabledGuard := NewSubmitGuard(nil, nil)
	if doneID, lease, acquired, err := disabledGuard.Begin(ctx, ""); err != nil {
		t.Fatalf("empty key Begin() error = %v", err)
	} else if doneID != "" || lease != nil || !acquired {
		t.Fatalf("empty key Begin() = doneID=%q lease=%+v acquired=%v, want allow", doneID, lease, acquired)
	}
	if doneID, lease, acquired, err := disabledGuard.Begin(ctx, "req-disabled"); err != nil {
		t.Fatalf("disabled guard Begin() error = %v", err)
	} else if doneID != "" || lease != nil || !acquired {
		t.Fatalf("disabled guard Begin() = doneID=%q lease=%+v acquired=%v, want allow", doneID, lease, acquired)
	}
}

func TestSubmitGuardUsesInjectedObserver(t *testing.T) {
	mr := miniredis.RunT(t)
	opsClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lockClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = opsClient.Close()
		_ = lockClient.Close()
	})

	opsHandle := &redisruntime.Handle{
		Family:  redisruntime.FamilyOps,
		Client:  opsClient,
		Builder: keyspace.NewBuilderWithNamespace("ops:runtime"),
	}
	lockHandle := &redisruntime.Handle{
		Family:  redisruntime.FamilyLock,
		Client:  lockClient,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
	}
	observer := &submitGuardRecordingObserver{}
	instanceA := NewSubmitGuardWithObserver(opsHandle, redisadapter.NewManager("collection-server-a", "lock_lease", lockHandle), observer)
	instanceB := NewSubmitGuardWithObserver(opsHandle, redisadapter.NewManager("collection-server-b", "lock_lease", lockHandle), observer)

	_, lease, acquired, err := instanceA.Begin(context.Background(), "req-observed")
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if !acquired || lease == nil {
		t.Fatalf("Begin() acquired=%v lease=%+v, want lock", acquired, lease)
	}
	if _, leaseB, acquired, err := instanceB.Begin(context.Background(), "req-observed"); err != nil {
		t.Fatalf("contention Begin() error = %v", err)
	} else if acquired || leaseB != nil {
		t.Fatalf("contention Begin() acquired=%v lease=%+v, want contention", acquired, leaseB)
	}
	if err := instanceA.Complete(context.Background(), "req-observed", lease, "answersheet-observed"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if doneID, _, acquired, err := instanceB.Begin(context.Background(), "req-observed"); err != nil {
		t.Fatalf("done marker Begin() error = %v", err)
	} else if doneID != "answersheet-observed" || acquired {
		t.Fatalf("done marker Begin() doneID=%q acquired=%v, want idempotency hit", doneID, acquired)
	}

	degraded := NewSubmitGuardWithObserver(nil, nil, observer)
	if _, _, acquired, err := degraded.Begin(context.Background(), "req-degraded"); err != nil {
		t.Fatalf("degraded Begin() error = %v", err)
	} else if !acquired {
		t.Fatal("degraded Begin() should fail open")
	}

	closedOpsClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if err := closedOpsClient.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	errorGuard := NewSubmitGuardWithObserver(&redisruntime.Handle{
		Family:  redisruntime.FamilyOps,
		Client:  closedOpsClient,
		Builder: keyspace.NewBuilderWithNamespace("ops:runtime"),
	}, nil, observer)
	if _, _, _, err := errorGuard.Begin(context.Background(), "req-error"); err == nil {
		t.Fatal("expected closed Redis client error")
	}

	for _, outcome := range []resilienceplane.Outcome{
		resilienceplane.OutcomeLockAcquired,
		resilienceplane.OutcomeLockContention,
		resilienceplane.OutcomeIdempotencyHit,
		resilienceplane.OutcomeDegradedOpen,
		resilienceplane.OutcomeLockError,
	} {
		if !observer.has(outcome) {
			t.Fatalf("observer missing outcome %s in %#v", outcome, observer.decisions)
		}
	}
}

type submitGuardRecordingObserver struct {
	decisions []resilienceplane.Decision
}

func (r *submitGuardRecordingObserver) ObserveDecision(_ context.Context, decision resilienceplane.Decision) {
	r.decisions = append(r.decisions, decision)
}

func (r *submitGuardRecordingObserver) has(outcome resilienceplane.Outcome) bool {
	for _, decision := range r.decisions {
		if decision.Outcome == outcome {
			return true
		}
	}
	return false
}

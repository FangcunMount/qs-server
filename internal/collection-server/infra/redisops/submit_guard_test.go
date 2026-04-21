package redisops

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
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

	opsHandle := &redisplane.Handle{
		Family:  redisplane.FamilyOps,
		Client:  opsClient,
		Builder: rediskey.NewBuilderWithNamespace("ops:runtime"),
	}
	lockHandle := &redisplane.Handle{
		Family:  redisplane.FamilyLock,
		Client:  lockClient,
		Builder: rediskey.NewBuilderWithNamespace("cache:lock"),
	}

	instanceA := NewSubmitGuard(opsHandle, redislock.NewManager("collection-server-a", "lock_lease", lockHandle))
	instanceB := NewSubmitGuard(opsHandle, redislock.NewManager("collection-server-b", "lock_lease", lockHandle))

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

func TestSubmitGuardReleasesInFlightLeaseOnAbort(t *testing.T) {
	mr := miniredis.RunT(t)
	opsClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lockClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = opsClient.Close()
		_ = lockClient.Close()
	})

	guard := NewSubmitGuard(
		&redisplane.Handle{
			Family:  redisplane.FamilyOps,
			Client:  opsClient,
			Builder: rediskey.NewBuilderWithNamespace("ops:runtime"),
		},
		redislock.NewManager("collection-server", "lock_lease", &redisplane.Handle{
			Family:  redisplane.FamilyLock,
			Client:  lockClient,
			Builder: rediskey.NewBuilderWithNamespace("cache:lock"),
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

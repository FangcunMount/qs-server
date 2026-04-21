package redislock

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestManagerAcquireReleaseAndContention(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	manager := NewManager("worker", "lock_lease", &redisplane.Handle{
		Family:  redisplane.FamilyLock,
		Client:  client,
		Builder: rediskey.NewBuilderWithNamespace("cache:lock"),
	})
	identity := Identity{Name: "plan_scheduler_leader", Key: "plan-scheduler:leader"}

	lease1, acquired, err := manager.Acquire(context.Background(), identity, 30*time.Second)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if !acquired || lease1 == nil {
		t.Fatalf("expected first acquire success, got acquired=%v lease=%+v", acquired, lease1)
	}

	if _, acquired, err := manager.Acquire(context.Background(), identity, 30*time.Second); err != nil {
		t.Fatalf("second acquire failed: %v", err)
	} else if acquired {
		t.Fatal("expected lock contention on second acquire")
	}

	if err := manager.Release(context.Background(), identity, &Lease{
		Key:   lease1.Key,
		Token: "wrong-token",
	}); err != nil {
		t.Fatalf("release with wrong token returned error: %v", err)
	}
	if _, acquired, err := manager.Acquire(context.Background(), identity, 30*time.Second); err != nil {
		t.Fatalf("acquire after wrong release failed: %v", err)
	} else if acquired {
		t.Fatal("expected wrong-token release to keep lock held")
	}

	if err := manager.Release(context.Background(), identity, lease1); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if _, acquired, err := manager.Acquire(context.Background(), identity, 30*time.Second); err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	} else if !acquired {
		t.Fatal("expected acquire success after release")
	}
}

func TestManagerAcquireSpecUsesSpecNameAndDefaultTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	manager := NewManager("worker", "lock_lease", &redisplane.Handle{
		Family:  redisplane.FamilyLock,
		Client:  client,
		Builder: rediskey.NewBuilderWithNamespace("cache:lock"),
	})

	lease, acquired, err := manager.AcquireSpec(context.Background(), Specs.PlanSchedulerLeader, "qs:plan-scheduler:test")
	if err != nil {
		t.Fatalf("AcquireSpec() error = %v", err)
	}
	if !acquired || lease == nil {
		t.Fatalf("AcquireSpec() got acquired=%v lease=%+v, want acquired lock", acquired, lease)
	}
	if ttl := mr.TTL(lease.Key); ttl != Specs.PlanSchedulerLeader.DefaultTTL {
		t.Fatalf("ttl = %s, want %s", ttl, Specs.PlanSchedulerLeader.DefaultTTL)
	}

	if err := manager.ReleaseSpec(context.Background(), Specs.PlanSchedulerLeader, "qs:plan-scheduler:test", lease); err != nil {
		t.Fatalf("ReleaseSpec() error = %v", err)
	}
}

func TestManagerLockLeaseExpiresAfterTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	manager := NewManager("collection-server", "lock_lease", &redisplane.Handle{
		Family:  redisplane.FamilyLock,
		Client:  client,
		Builder: rediskey.NewBuilderWithNamespace("cache:lock"),
	})
	identity := Identity{Name: "collection_submit", Key: "submit:idempotency:abc:lock"}

	if _, acquired, err := manager.Acquire(context.Background(), identity, 5*time.Second); err != nil {
		t.Fatalf("initial acquire failed: %v", err)
	} else if !acquired {
		t.Fatal("expected initial acquire success")
	}

	mr.FastForward(6 * time.Second)

	if _, acquired, err := manager.Acquire(context.Background(), identity, 5*time.Second); err != nil {
		t.Fatalf("acquire after ttl expiry failed: %v", err)
	} else if !acquired {
		t.Fatal("expected acquire success after ttl expiry")
	}
}

func TestManagerReleaseSpecWithWrongTokenDoesNotUnlock(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	manager := NewManager("collection-server", "lock_lease", &redisplane.Handle{
		Family:  redisplane.FamilyLock,
		Client:  client,
		Builder: rediskey.NewBuilderWithNamespace("cache:lock"),
	})

	lease, acquired, err := manager.AcquireSpec(context.Background(), Specs.CollectionSubmit, "submit:idempotency:req-1:lock")
	if err != nil {
		t.Fatalf("AcquireSpec() error = %v", err)
	}
	if !acquired || lease == nil {
		t.Fatalf("AcquireSpec() got acquired=%v lease=%+v, want acquired lock", acquired, lease)
	}

	if err := manager.ReleaseSpec(context.Background(), Specs.CollectionSubmit, "submit:idempotency:req-1:lock", &Lease{
		Key:   lease.Key,
		Token: "wrong-token",
	}); err != nil {
		t.Fatalf("ReleaseSpec() with wrong token error = %v", err)
	}

	if _, acquired, err := manager.AcquireSpec(context.Background(), Specs.CollectionSubmit, "submit:idempotency:req-1:lock"); err != nil {
		t.Fatalf("AcquireSpec() after wrong release error = %v", err)
	} else if acquired {
		t.Fatal("expected lock to remain held after wrong-token release")
	}
}

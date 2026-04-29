package redisadapter

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestManagerAcquireReleaseAndContention(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	manager := NewManager("worker", "lock_lease", &cacheplane.Handle{
		Family:  cacheplane.FamilyLock,
		Client:  client,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
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

	manager := NewManager("worker", "lock_lease", &cacheplane.Handle{
		Family:  cacheplane.FamilyLock,
		Client:  client,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
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

func TestManagerAcquireSpecRejectsInvalidSpec(t *testing.T) {
	manager := NewManager("worker", "lock_lease", &cacheplane.Handle{
		Family:  cacheplane.FamilyLock,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
	})

	if _, acquired, err := manager.AcquireSpec(context.Background(), Spec{
		DefaultTTL: time.Second,
	}, "invalid:name"); err == nil {
		t.Fatal("expected empty spec name to be rejected")
	} else if acquired {
		t.Fatal("expected invalid spec to not acquire lock")
	}

	if _, acquired, err := manager.AcquireSpec(context.Background(), Spec{
		Name: "invalid_ttl",
	}, "invalid:ttl"); err == nil {
		t.Fatal("expected empty ttl to be rejected")
	} else if acquired {
		t.Fatal("expected invalid ttl to not acquire lock")
	}
}

func TestManagerReleaseNoOpsWhenUnavailable(t *testing.T) {
	ctx := context.Background()
	lease := &Lease{Key: "cache:lock:any", Token: "token"}

	var nilManager *Manager
	if err := nilManager.Release(ctx, Identity{}, lease); err != nil {
		t.Fatalf("nil manager Release() error = %v", err)
	}
	if err := nilManager.ReleaseSpec(ctx, Specs.AnswersheetProcessing, "answersheet:processing:1", lease); err != nil {
		t.Fatalf("nil manager ReleaseSpec() error = %v", err)
	}

	unavailableManager := NewManager("worker", "lock_lease", nil)
	if err := unavailableManager.Release(ctx, Identity{Name: "answersheet_processing"}, lease); err != nil {
		t.Fatalf("unavailable manager Release() error = %v", err)
	}
	if err := unavailableManager.Release(ctx, Identity{Name: "answersheet_processing"}, nil); err != nil {
		t.Fatalf("nil lease Release() error = %v", err)
	}
}

func TestManagerLockLeaseExpiresAfterTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	manager := NewManager("collection-server", "lock_lease", &cacheplane.Handle{
		Family:  cacheplane.FamilyLock,
		Client:  client,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
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

	manager := NewManager("collection-server", "lock_lease", &cacheplane.Handle{
		Family:  cacheplane.FamilyLock,
		Client:  client,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
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

func TestManagerUsesInjectedObserver(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	observer := &redislockRecordingObserver{}
	manager := NewManagerWithObserver("worker", "lock_lease", &cacheplane.Handle{
		Family:  cacheplane.FamilyLock,
		Client:  client,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
	}, observer)
	identity := Identity{Name: "answersheet_processing", Key: "answersheet:processing:1"}

	lease, acquired, err := manager.Acquire(context.Background(), identity, time.Minute)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if !acquired || lease == nil {
		t.Fatalf("Acquire() acquired=%v lease=%+v, want lock", acquired, lease)
	}
	if _, acquired, err := manager.Acquire(context.Background(), identity, time.Minute); err != nil {
		t.Fatalf("contention Acquire() error = %v", err)
	} else if acquired {
		t.Fatal("contention Acquire() acquired lock, want false")
	}
	if err := manager.Release(context.Background(), identity, lease); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if _, _, err := manager.Acquire(context.Background(), Identity{}, time.Minute); err == nil {
		t.Fatal("expected invalid identity acquire error")
	}

	for _, outcome := range []resilienceplane.Outcome{
		resilienceplane.OutcomeLockAcquired,
		resilienceplane.OutcomeLockContention,
		resilienceplane.OutcomeLockReleased,
		resilienceplane.OutcomeLockError,
	} {
		if !observer.has(outcome) {
			t.Fatalf("observer missing outcome %s in %#v", outcome, observer.decisions)
		}
	}
}

type redislockRecordingObserver struct {
	decisions []resilienceplane.Decision
}

func (r *redislockRecordingObserver) ObserveDecision(_ context.Context, decision resilienceplane.Decision) {
	r.decisions = append(r.decisions, decision)
}

func (r *redislockRecordingObserver) has(outcome resilienceplane.Outcome) bool {
	for _, decision := range r.decisions {
		if decision.Outcome == outcome {
			return true
		}
	}
	return false
}

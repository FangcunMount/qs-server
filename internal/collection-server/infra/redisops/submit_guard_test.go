package redisops

import (
	"context"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestSubmitGuardRunUsesAdvisoryLeaseWithoutCachingResult(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	opsHandle := &redisruntime.Handle{Family: redisruntime.FamilyOps, Client: client, Builder: keyspace.NewBuilderWithNamespace("ops:runtime")}
	lockHandle := &redisruntime.Handle{Family: redisruntime.FamilyLock, Client: client, Builder: keyspace.NewBuilderWithNamespace("cache:lock"), Configured: true, Available: true}
	locks := locksubsystem.New(locksubsystem.Options{Component: "collection-server", Handle: lockHandle})
	guard := NewSubmitGuardWithRunner(opsHandle, locks)
	lockKey := lockHandle.Builder.BuildLockKey(submitInflightKey("submit-1"))

	calls := 0
	for range 2 {
		value, acquired, err := guard.Run(t.Context(), "submit-1", func(context.Context) (string, error) {
			calls++
			if !mr.Exists(lockKey) {
				t.Fatal("advisory lease must be held while body executes")
			}
			return "42", nil
		})
		if err != nil || !acquired || value != "42" {
			t.Fatalf("Run() = value=%q acquired=%v err=%v", value, acquired, err)
		}
	}
	if calls != 2 {
		t.Fatalf("body calls = %d, want 2; Redis must not cache the final result", calls)
	}
	for _, key := range mr.Keys() {
		if strings.HasSuffix(key, ":done") {
			t.Fatalf("Redis final-result marker must not be written: %s", key)
		}
	}
}

func TestSubmitGuardDegradesOpenWithoutLeaseRuntime(t *testing.T) {
	guard := NewSubmitGuardWithRunner(nil, nil)
	called := false
	value, acquired, err := guard.Run(t.Context(), "submit-2", func(context.Context) (string, error) {
		called = true
		return "43", nil
	})
	if err != nil || !acquired || !called || value != "43" {
		t.Fatalf("Run() = value=%q acquired=%v called=%v err=%v", value, acquired, called, err)
	}
}

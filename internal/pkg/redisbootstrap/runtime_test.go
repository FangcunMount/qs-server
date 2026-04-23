package redisbootstrap

import (
	"context"
	"errors"
	"testing"

	cbdatabase "github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type resolverStub struct {
	defaultClient  redis.UniversalClient
	defaultErr     error
	profileClients map[string]redis.UniversalClient
	profileErrs    map[string]error
	statuses       map[string]cbdatabase.RedisProfileStatus
}

func (s resolverStub) GetRedisClient() (redis.UniversalClient, error) {
	return s.defaultClient, s.defaultErr
}

func (s resolverStub) GetRedisClientByProfile(profile string) (redis.UniversalClient, error) {
	if err, ok := s.profileErrs[profile]; ok {
		return nil, err
	}
	return s.profileClients[profile], nil
}

func (s resolverStub) GetRedisProfileStatus(profile string) cbdatabase.RedisProfileStatus {
	if status, ok := s.statuses[profile]; ok {
		return status
	}
	return cbdatabase.RedisProfileStatus{Name: profile, State: cbdatabase.RedisProfileStateMissing}
}

func TestBuildRuntimeCreatesFamilyHandlesAndLockManager(t *testing.T) {
	mr := miniredis.RunT(t)
	defaultClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lockClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = defaultClient.Close()
		_ = lockClient.Close()
	})

	bundle := BuildRuntime(context.Background(), Options{
		Component: "worker",
		RuntimeOptions: &genericoptions.RedisRuntimeOptions{
			Namespace: "qs",
		},
		Defaults: map[redisplane.Family]redisplane.Route{
			redisplane.FamilyLock: {
				RedisProfile:         "lock_cache",
				NamespaceSuffix:      "cache:lock",
				AllowFallbackDefault: true,
			},
		},
		Resolver: resolverStub{
			defaultClient: defaultClient,
			profileClients: map[string]redis.UniversalClient{
				"lock_cache": lockClient,
			},
			statuses: map[string]cbdatabase.RedisProfileStatus{
				"lock_cache": {Name: "lock_cache", State: cbdatabase.RedisProfileStateAvailable},
			},
		},
		LockName: "lock_lease",
	})

	if bundle == nil {
		t.Fatal("bundle = nil, want runtime bundle")
	}
	if bundle.StatusRegistry == nil || bundle.Runtime == nil || bundle.LockManager == nil {
		t.Fatalf("runtime outputs missing: %#v", bundle)
	}
	if got := bundle.Builder(redisplane.FamilyLock).BuildLockKey("answersheet:1"); got != "qs:cache:lock:answersheet:1" {
		t.Fatalf("lock key = %q, want qs:cache:lock:answersheet:1", got)
	}
	handle := bundle.Handle(redisplane.FamilyLock)
	if handle == nil || handle.Client != lockClient {
		t.Fatalf("lock handle = %#v, want named-profile handle", handle)
	}
}

func TestBuildRuntimeKeepsFallbackDefaultSemantics(t *testing.T) {
	mr := miniredis.RunT(t)
	defaultClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = defaultClient.Close() })

	bundle := BuildRuntime(context.Background(), Options{
		Component: "collection-server",
		Defaults: map[redisplane.Family]redisplane.Route{
			redisplane.FamilyOps: {
				RedisProfile:         "ops_runtime",
				NamespaceSuffix:      "ops:runtime",
				AllowFallbackDefault: true,
			},
		},
		Resolver: resolverStub{
			defaultClient:  defaultClient,
			profileClients: map[string]redis.UniversalClient{},
			statuses:       map[string]cbdatabase.RedisProfileStatus{},
		},
	})

	handle := bundle.Handle(redisplane.FamilyOps)
	if handle == nil {
		t.Fatal("ops handle = nil, want fallback handle")
	}
	if handle.Client != defaultClient {
		t.Fatal("ops handle client = non-default, want default fallback client")
	}
	if handle.Mode != cacheobservability.FamilyModeFallbackDefault {
		t.Fatalf("mode = %q, want %q", handle.Mode, cacheobservability.FamilyModeFallbackDefault)
	}
}

func TestBuildRuntimeKeepsDegradedSemantics(t *testing.T) {
	bundle := BuildRuntime(context.Background(), Options{
		Component: "apiserver",
		Defaults: map[redisplane.Family]redisplane.Route{
			redisplane.FamilyQuery: {
				RedisProfile:         "query_cache",
				NamespaceSuffix:      "cache:query",
				AllowFallbackDefault: false,
			},
		},
		Resolver: resolverStub{
			statuses: map[string]cbdatabase.RedisProfileStatus{
				"query_cache": {
					Name:  "query_cache",
					State: cbdatabase.RedisProfileStateUnavailable,
					Err:   errors.New("dial failed"),
				},
			},
		},
	})

	handle := bundle.Handle(redisplane.FamilyQuery)
	if handle == nil {
		t.Fatal("query handle = nil, want degraded handle")
	}
	if !handle.Degraded || handle.Available {
		t.Fatalf("query handle = %#v, want degraded unavailable handle", handle)
	}
	if handle.Mode != cacheobservability.FamilyModeDegraded {
		t.Fatalf("mode = %q, want %q", handle.Mode, cacheobservability.FamilyModeDegraded)
	}
}

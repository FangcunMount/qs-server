package cachebootstrap

import (
	"context"
	"testing"

	cbdatabase "github.com/FangcunMount/component-base/pkg/database"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	redis "github.com/redis/go-redis/v9"
)

type fakeResolver struct {
	profiles map[string]redis.UniversalClient
}

func (r fakeResolver) GetRedisClient() (redis.UniversalClient, error) {
	return nil, nil
}

func (r fakeResolver) GetRedisClientByProfile(profile string) (redis.UniversalClient, error) {
	if client, ok := r.profiles[profile]; ok {
		return client, nil
	}
	return nil, nil
}

func (r fakeResolver) GetRedisProfileStatus(profile string) cbdatabase.RedisProfileStatus {
	if _, ok := r.profiles[profile]; ok {
		return cbdatabase.RedisProfileStatus{State: cbdatabase.RedisProfileStateAvailable}
	}
	return cbdatabase.RedisProfileStatus{State: cbdatabase.RedisProfileStateMissing}
}

func TestSubsystemOwnsRuntimeAndGovernanceLifecycle(t *testing.T) {
	subsystem := NewSubsystem(
		"apiserver",
		fakeResolver{},
		&genericoptions.RedisRuntimeOptions{},
		CacheOptions{Warmup: WarmupOptions{Enable: true}},
	)
	if subsystem == nil {
		t.Fatal("subsystem = nil, want non-nil")
	}
	if subsystem.Runtime() == nil {
		t.Fatal("runtime = nil, want initialized runtime")
	}
	if subsystem.StatusRegistry() == nil {
		t.Fatal("status registry = nil, want initialized registry")
	}
	if subsystem.LockManager() == nil {
		t.Fatal("lock manager = nil, want initialized lock manager")
	}
	if subsystem.StatusService() != nil {
		t.Fatal("status service initialized before BindGovernance, want nil")
	}

	subsystem.BindGovernance(GovernanceBindings{})
	if subsystem.WarmupCoordinator() == nil {
		t.Fatal("warmup coordinator = nil after BindGovernance")
	}
	if subsystem.StatusService() == nil {
		t.Fatal("status service = nil after BindGovernance")
	}

	snapshot, err := subsystem.StatusService().GetRuntime(context.Background())
	if err != nil {
		t.Fatalf("GetRuntime() error = %v", err)
	}
	if snapshot == nil || snapshot.Component != "apiserver" {
		t.Fatalf("runtime snapshot = %#v, want component apiserver", snapshot)
	}
}

func TestSubsystemReturnsFamilyScopedBuilders(t *testing.T) {
	subsystem := NewSubsystem(
		"apiserver",
		fakeResolver{},
		&genericoptions.RedisRuntimeOptions{
			Namespace: "prod:cache",
			Families: map[string]*genericoptions.RedisRuntimeFamilyRoute{
				string(redisplane.FamilyObject): {NamespaceSuffix: "object"},
				string(redisplane.FamilyQuery):  {NamespaceSuffix: "query"},
			},
		},
		CacheOptions{},
	)

	objectBuilder := subsystem.Builder(redisplane.FamilyObject)
	queryBuilder := subsystem.Builder(redisplane.FamilyQuery)
	if objectBuilder == nil || queryBuilder == nil {
		t.Fatal("builders = nil, want family-scoped builders")
	}
	if got := objectBuilder.BuildTesteeInfoKey(42); got != "prod:cache:object:testee:info:42" {
		t.Fatalf("object builder key = %q, want prod:cache:object:testee:info:42", got)
	}
	if got := queryBuilder.BuildQueryVersionKey("stats:query", "system:1"); got != "prod:cache:query:query:version:stats:query:system:1" {
		t.Fatalf("query builder key = %q, want prod:cache:query:query:version:stats:query:system:1", got)
	}
}

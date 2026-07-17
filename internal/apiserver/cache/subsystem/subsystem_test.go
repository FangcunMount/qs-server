package cachebootstrap

import (
	"context"
	"testing"

	cbdatabase "github.com/FangcunMount/component-base/pkg/database"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
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
				string(redisruntime.FamilyObject): {NamespaceSuffix: "object"},
				string(redisruntime.FamilyQuery):  {NamespaceSuffix: "query"},
			},
		},
		CacheOptions{},
	)

	objectBuilder := subsystem.Builder(redisruntime.FamilyObject)
	queryBuilder := subsystem.Builder(redisruntime.FamilyQuery)
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

func TestSubsystemUsesSharedRedisRuntimeBundle(t *testing.T) {
	runtimeBundle := cacheplanebootstrap.BuildRuntime(context.Background(), cacheplanebootstrap.Options{
		Component: "apiserver",
		RuntimeOptions: &genericoptions.RedisRuntimeOptions{
			Namespace: "prod:cache",
			Families: map[string]*genericoptions.RedisRuntimeFamilyRoute{
				string(redisruntime.FamilyMeta): {NamespaceSuffix: "meta"},
				string(redisruntime.FamilyLock): {NamespaceSuffix: "lock"},
			},
		},
		Resolver: fakeResolver{},
	})

	subsystem := NewSubsystemFromRuntime(runtimeBundle, CacheOptions{})
	if subsystem == nil {
		t.Fatal("subsystem = nil, want non-nil")
	}
	if subsystem.Runtime() != runtimeBundle.Runtime {
		t.Fatal("subsystem runtime did not use shared runtime bundle")
	}
	if subsystem.StatusRegistry() != runtimeBundle.StatusRegistry {
		t.Fatal("subsystem status registry did not use shared runtime bundle")
	}
	if got := subsystem.Builder(redisruntime.FamilyMeta).BuildQueryVersionKey("stats", "system"); got != "prod:cache:meta:query:version:stats:system" {
		t.Fatalf("meta builder key = %q, want prod:cache:meta:query:version:stats:system", got)
	}
}

func TestSubsystemStartCloseAreIdempotent(t *testing.T) {
	subsystem := NewSubsystem("apiserver", fakeResolver{}, &genericoptions.RedisRuntimeOptions{}, CacheOptions{})
	subsystem.BindGovernance(GovernanceBindings{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := subsystem.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	firstCancel := subsystem.cancel
	if firstCancel == nil {
		t.Fatal("Start() did not install lifecycle cancel")
	}
	if err := subsystem.Start(ctx); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if subsystem.cancel == nil {
		t.Fatal("second Start() cleared lifecycle cancel")
	}
	if err := subsystem.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := subsystem.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if subsystem.started || subsystem.cancel != nil {
		t.Fatalf("closed lifecycle state = started:%v cancel:%v", subsystem.started, subsystem.cancel != nil)
	}
}

func TestSubsystemStartDegradesWithoutRedisOrNotifier(t *testing.T) {
	subsystem := NewSubsystem("apiserver", fakeResolver{}, &genericoptions.RedisRuntimeOptions{}, CacheOptions{Warmup: WarmupOptions{Enable: true}})
	subsystem.BindGovernance(GovernanceBindings{})
	if err := subsystem.Start(context.Background()); err != nil {
		t.Fatalf("degraded Start() error = %v", err)
	}
	if err := subsystem.Close(); err != nil {
		t.Fatalf("degraded Close() error = %v", err)
	}
}

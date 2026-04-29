package cacheplane

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type runtimeResolverStub struct {
	defaultClient  redis.UniversalClient
	defaultErr     error
	profileClients map[string]redis.UniversalClient
	profileErrs    map[string]error
	statuses       map[string]database.RedisProfileStatus
}

func (s *runtimeResolverStub) GetRedisClient() (redis.UniversalClient, error) {
	return s.defaultClient, s.defaultErr
}

func (s *runtimeResolverStub) GetRedisClientByProfile(profile string) (redis.UniversalClient, error) {
	if err, ok := s.profileErrs[profile]; ok {
		return nil, err
	}
	return s.profileClients[profile], nil
}

func (s *runtimeResolverStub) GetRedisProfileStatus(profile string) database.RedisProfileStatus {
	if status, ok := s.statuses[profile]; ok {
		return status
	}
	return database.RedisProfileStatus{Name: profile, State: database.RedisProfileStateMissing}
}

func TestRuntimeHandleUsesNamedProfileAndNamespace(t *testing.T) {
	mr := miniredis.RunT(t)
	defaultClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	profileClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = defaultClient.Close()
		_ = profileClient.Close()
	})

	resolver := &runtimeResolverStub{
		defaultClient: defaultClient,
		profileClients: map[string]redis.UniversalClient{
			"static_cache": profileClient,
		},
		statuses: map[string]database.RedisProfileStatus{
			"static_cache": {Name: "static_cache", State: database.RedisProfileStateAvailable},
		},
	}
	registry := observability.NewFamilyStatusRegistry("apiserver")
	runtime := NewRuntime("apiserver", resolver, NewCatalog("qs", map[Family]Route{
		FamilyStatic: {
			RedisProfile:         "static_cache",
			NamespaceSuffix:      "cache:static",
			AllowFallbackDefault: true,
			AllowWarmup:          true,
		},
	}), registry)

	handle := runtime.Handle(context.Background(), FamilyStatic)
	if handle == nil {
		t.Fatal("expected handle")
	}
	if handle.Client != profileClient {
		t.Fatal("expected named-profile redis client")
	}
	if handle.Mode != observability.FamilyModeNamedProfile {
		t.Fatalf("mode = %q, want %q", handle.Mode, observability.FamilyModeNamedProfile)
	}
	if handle.Namespace != "qs:cache:static" {
		t.Fatalf("namespace = %q, want %q", handle.Namespace, "qs:cache:static")
	}
	if got := handle.Builder.BuildLockKey("version"); got != "qs:cache:static:version" {
		t.Fatalf("builder key = %q, want %q", got, "qs:cache:static:version")
	}

	snapshot := registry.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot length = %d, want 1", len(snapshot))
	}
	if snapshot[0].Family != string(FamilyStatic) {
		t.Fatalf("family = %q, want %q", snapshot[0].Family, FamilyStatic)
	}
	if !snapshot[0].Available || snapshot[0].Degraded {
		t.Fatalf("unexpected family status: %+v", snapshot[0])
	}
}

func TestRuntimeHandleFallsBackToDefaultProfileWhenNamedProfileMissing(t *testing.T) {
	mr := miniredis.RunT(t)
	defaultClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = defaultClient.Close()
	})

	resolver := &runtimeResolverStub{
		defaultClient:  defaultClient,
		profileClients: map[string]redis.UniversalClient{},
		statuses:       map[string]database.RedisProfileStatus{},
	}
	registry := observability.NewFamilyStatusRegistry("worker")
	runtime := NewRuntime("worker", resolver, NewCatalog("qs", map[Family]Route{
		FamilyLock: {
			RedisProfile:         "lock_cache",
			NamespaceSuffix:      "cache:lock",
			AllowFallbackDefault: true,
		},
	}), registry)

	handle := runtime.Handle(context.Background(), FamilyLock)
	if handle == nil {
		t.Fatal("expected handle")
	}
	if handle.Client != defaultClient {
		t.Fatal("expected default redis client fallback")
	}
	if handle.Mode != observability.FamilyModeFallbackDefault {
		t.Fatalf("mode = %q, want %q", handle.Mode, observability.FamilyModeFallbackDefault)
	}
	if handle.Configured {
		t.Fatal("expected missing named profile to remain unconfigured")
	}
	if !handle.Available || handle.Degraded {
		t.Fatalf("unexpected fallback handle state: %+v", handle)
	}
}

func TestRuntimeHandleMarksUnavailableNamedProfileAsDegraded(t *testing.T) {
	registry := observability.NewFamilyStatusRegistry("collection-server")
	resolver := &runtimeResolverStub{
		statuses: map[string]database.RedisProfileStatus{
			"ops_runtime": {
				Name:  "ops_runtime",
				State: database.RedisProfileStateUnavailable,
				Err:   errors.New("dial tcp: connection refused"),
			},
		},
	}
	runtime := NewRuntime("collection-server", resolver, NewCatalog("qs", map[Family]Route{
		FamilyOps: {
			RedisProfile:         "ops_runtime",
			NamespaceSuffix:      "ops:runtime",
			AllowFallbackDefault: false,
		},
	}), registry)

	handle := runtime.Handle(context.Background(), FamilyOps)
	if handle == nil {
		t.Fatal("expected handle")
	}
	if !handle.Degraded {
		t.Fatal("expected degraded handle")
	}
	if handle.Available {
		t.Fatal("expected unavailable handle")
	}
	if handle.Mode != observability.FamilyModeDegraded {
		t.Fatalf("mode = %q, want %q", handle.Mode, observability.FamilyModeDegraded)
	}
	if handle.LastError == nil {
		t.Fatal("expected degraded handle error")
	}
}

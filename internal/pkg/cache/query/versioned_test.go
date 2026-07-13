package query

import (
	"context"
	"testing"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	cacheobserve "github.com/FangcunMount/qs-server/internal/pkg/cache/observe"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestVersionedQueryCacheObserverUsesInjectedComponent(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	registry := observability.NewFamilyStatusRegistry("query-observer")
	registry.Update(observability.FamilyStatus{
		Component: "query-observer",
		Family:    "query_result",
		Available: false,
		Degraded:  true,
		Mode:      observability.FamilyModeDegraded,
	})

	cache := NewVersioned(VersionedOptions{
		Store:      redisstore.NewStore(client),
		Version:    NewStaticVersionTokenStore(0),
		Capability: "assessment_list",
		Policies: sharedcache.NewRegistry(sharedcache.EffectiveCapability{
			Capability: "assessment_list", Policy: sharedcache.Policy{TTL: time.Minute},
		}),
		Observer: cacheobserve.NewPrometheus("query_result", "assessment_list", testFamilyObserver{registry: registry}),
	})
	cache.Set(context.Background(), "query:version:assessment:list:42", func(version uint64) string {
		return "query:assessment:list:42:v0"
	}, map[string]string{"ok": "true"})

	snapshot := observability.SnapshotForComponent("query-observer", registry)
	if !snapshot.Summary.Ready {
		t.Fatalf("runtime summary ready = false, want true after observed success: %#v", snapshot.Summary)
	}
}

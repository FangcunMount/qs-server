package cache

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestVersionedQueryCacheObserverUsesInjectedComponent(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	registry := cacheobservability.NewFamilyStatusRegistry("query-observer")
	registry.Update(cacheobservability.FamilyStatus{
		Component: "query-observer",
		Family:    string(cachepolicy.FamilyFor(cachepolicy.PolicyAssessmentList)),
		Available: false,
		Degraded:  true,
		Mode:      cacheobservability.FamilyModeDegraded,
	})

	cache := NewVersionedQueryCacheWithObserver(
		NewRedisCache(client),
		NewStaticVersionTokenStore(0),
		cachepolicy.PolicyAssessmentList,
		cachepolicy.CachePolicy{TTL: time.Minute},
		time.Minute,
		nil,
		NewObserver("query-observer"),
	)
	cache.Set(context.Background(), "query:version:assessment:list:42", func(version uint64) string {
		return "query:assessment:list:42:v0"
	}, map[string]string{"ok": "true"})

	snapshot := cacheobservability.SnapshotForComponent("query-observer", registry)
	if !snapshot.Summary.Ready {
		t.Fatalf("runtime summary ready = false, want true after observed success: %#v", snapshot.Summary)
	}
}

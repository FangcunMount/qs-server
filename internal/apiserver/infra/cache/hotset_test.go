package cache

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisHotsetStoreObserverUsesInjectedComponent(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	registry := cacheobservability.NewFamilyStatusRegistry("hotset-observer")
	registry.Update(cacheobservability.FamilyStatus{
		Component: "hotset-observer",
		Family:    string(redisplane.FamilyMeta),
		Available: false,
		Degraded:  true,
		Mode:      cacheobservability.FamilyModeDegraded,
	})

	recorder := NewRedisHotsetStoreWithObserver(
		client,
		rediskey.NewBuilderWithNamespace("prod:cache:meta"),
		HotsetOptions{Enable: true, TopN: 10, MaxItemsPerKind: 20},
		NewObserver("hotset-observer"),
	)
	if recorder == nil {
		t.Fatal("recorder = nil, want enabled hotset recorder")
	}
	if err := recorder.Record(context.Background(), WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   WarmupKindStaticScaleList,
		Scope:  "SDS",
	}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	snapshot := cacheobservability.SnapshotForComponent("hotset-observer", registry)
	if !snapshot.Summary.Ready {
		t.Fatalf("runtime summary ready = false, want true after observed success: %#v", snapshot.Summary)
	}
}

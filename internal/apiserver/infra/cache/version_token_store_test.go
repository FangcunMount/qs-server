package cache

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisVersionTokenStoreCurrentAndBump(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), DB: 6})
	t.Cleanup(func() {
		_ = client.Close()
	})

	store := NewRedisVersionTokenStore(client)
	ctx := context.Background()

	version, err := store.Current(ctx, "cache:meta:query:version:assessment:list:42")
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if version != 0 {
		t.Fatalf("Current() = %d, want 0 for missing key", version)
	}

	version, err = store.Bump(ctx, "cache:meta:query:version:assessment:list:42")
	if err != nil {
		t.Fatalf("Bump() error = %v", err)
	}
	if version != 1 {
		t.Fatalf("Bump() = %d, want 1", version)
	}

	version, err = store.Current(ctx, "cache:meta:query:version:assessment:list:42")
	if err != nil {
		t.Fatalf("Current() after bump error = %v", err)
	}
	if version != 1 {
		t.Fatalf("Current() after bump = %d, want 1", version)
	}
}

func TestStaticVersionTokenStoreReturnsConfiguredVersion(t *testing.T) {
	store := NewStaticVersionTokenStore(7)

	version, err := store.Current(context.Background(), "query:version:stats:query:system:1")
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if version != 7 {
		t.Fatalf("Current() = %d, want 7", version)
	}

	version, err = store.Bump(context.Background(), "query:version:stats:query:system:1")
	if err != nil {
		t.Fatalf("Bump() error = %v", err)
	}
	if version != 7 {
		t.Fatalf("Bump() = %d, want 7", version)
	}
}

func TestRedisVersionTokenStoreObserverUsesInjectedComponent(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	registry := cacheobservability.NewFamilyStatusRegistry("token-observer")
	registry.Update(cacheobservability.FamilyStatus{
		Component: "token-observer",
		Family:    "meta_hotset",
		Available: false,
		Degraded:  true,
		Mode:      cacheobservability.FamilyModeDegraded,
	})

	store := NewRedisVersionTokenStoreWithKindAndObserver(client, "assessment:list", NewObserver("token-observer"))
	if _, err := store.Current(context.Background(), "query:version:assessment:list:42"); err != nil {
		t.Fatalf("Current() error = %v", err)
	}

	snapshot := cacheobservability.SnapshotForComponent("token-observer", registry)
	if !snapshot.Summary.Ready {
		t.Fatalf("runtime summary ready = false, want true after observed success: %#v", snapshot.Summary)
	}
}

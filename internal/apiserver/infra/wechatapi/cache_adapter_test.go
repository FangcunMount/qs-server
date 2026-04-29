package wechatapi

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisCacheAdapterUsesExplicitBuilderNamespace(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewRedisCacheAdapterWithBuilder(client, keyspace.NewBuilderWithNamespace("prod:cache:sdk"))
	if err := cache.Set("access_token", "token-1", time.Minute); err != nil {
		t.Fatalf("cache set failed: %v", err)
	}
	if !mr.Exists("prod:cache:sdk:wechat:cache:access_token") {
		t.Fatalf("expected sdk namespaced wechat cache key")
	}
	if got := cache.Get("access_token"); got != "token-1" {
		t.Fatalf("unexpected cache value: %v", got)
	}
}

func TestRedisCacheAdapterUsesInjectedObserver(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	registry := observability.NewFamilyStatusRegistry("wechat-sdk-test")
	registry.Update(observability.FamilyStatus{
		Component: "wechat-sdk-test",
		Family:    "sdk_token",
		Available: false,
		Degraded:  true,
		Mode:      observability.FamilyModeDegraded,
	})

	cache := NewRedisCacheAdapterWithBuilderAndObserver(
		client,
		keyspace.NewBuilderWithNamespace("prod:cache:sdk"),
		observability.NewComponentObserver("wechat-sdk-test"),
	)
	if err := cache.Set("access_token", "token-1", time.Minute); err != nil {
		t.Fatalf("cache set failed: %v", err)
	}

	snapshot := observability.SnapshotForComponent("wechat-sdk-test", registry)
	if !snapshot.Summary.Ready {
		t.Fatalf("runtime summary ready = false, want true after observed success: %#v", snapshot.Summary)
	}
}

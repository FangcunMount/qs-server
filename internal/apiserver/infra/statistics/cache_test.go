package statistics

import (
	"context"
	"testing"
	"time"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestStatisticsCacheUsesNamespacedQueryKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndPolicy(client, rediskey.NewBuilderWithNamespace("stats-test"), cachepolicy.CachePolicy{})
	ctx := context.Background()

	if err := cache.SetQueryCache(ctx, "system:1", "{\"ok\":true}", time.Minute); err != nil {
		t.Fatalf("set query cache failed: %v", err)
	}
	value, err := cache.GetQueryCache(ctx, "system:1")
	if err != nil {
		t.Fatalf("get query cache failed: %v", err)
	}
	if value != "{\"ok\":true}" {
		t.Fatalf("unexpected cache value: %s", value)
	}
	if !mr.Exists("stats-test:query:stats:query:system:1:v0") {
		t.Fatalf("expected versioned namespaced stats query key")
	}
	if mr.Exists("stats-test:query:version:stats:query:system:1") {
		t.Fatalf("unexpected version token key for static fallback store")
	}
}

func TestStatisticsCacheUsesExplicitBuilderNamespace(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndPolicy(client, rediskey.NewBuilderWithNamespace("prod:cache:query"), cachepolicy.CachePolicy{})
	ctx := context.Background()

	if err := cache.SetQueryCache(ctx, "system:1", "{\"ok\":true}", time.Minute); err != nil {
		t.Fatalf("set query cache failed: %v", err)
	}
	if !mr.Exists("prod:cache:query:query:stats:query:system:1:v0") {
		t.Fatalf("expected explicit namespaced versioned stats query key")
	}
}

func TestStatisticsCacheAppliesPolicyTTLAndCompression(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndPolicy(
		client,
		rediskey.NewBuilderWithNamespace("prod:cache:query"),
		cachepolicy.CachePolicy{
			TTL:      3 * time.Minute,
			Compress: cachepolicy.PolicySwitchEnabled,
		},
	)
	ctx := context.Background()

	if err := cache.SetQueryCache(ctx, "system:1", "{\"ok\":true}", 0); err != nil {
		t.Fatalf("set query cache failed: %v", err)
	}

	ttl := mr.TTL("prod:cache:query:query:stats:query:system:1:v0")
	if ttl <= 0 || ttl > 3*time.Minute {
		t.Fatalf("expected policy ttl to be applied, got %v", ttl)
	}

	value, err := cache.GetQueryCache(ctx, "system:1")
	if err != nil {
		t.Fatalf("get query cache failed: %v", err)
	}
	if value != "{\"ok\":true}" {
		t.Fatalf("unexpected cache value after compression roundtrip: %s", value)
	}
}

func TestStatisticsCacheDegradesRedisReadErrorToMiss(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:63999"})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndPolicy(client, rediskey.NewBuilderWithNamespace("prod:cache:query"), cachepolicy.CachePolicy{})
	value, err := cache.GetQueryCache(context.Background(), "system:1")
	if err != nil {
		t.Fatalf("GetQueryCache() error = %v", err)
	}
	if value != "" {
		t.Fatalf("GetQueryCache() value = %q, want empty miss", value)
	}
}

func TestStatisticsCacheSupportsMissingVersionTokenStoreKey(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderPolicyVersionStoreAndObserver(
		client,
		rediskey.NewBuilderWithNamespace("prod:cache:query"),
		cachepolicy.CachePolicy{TTL: time.Minute},
		nil,
		nil,
	)
	ctx := context.Background()

	if err := cache.SetQueryCache(ctx, "system:1", "{\"ok\":true}", time.Minute); err != nil {
		t.Fatalf("SetQueryCache() error = %v", err)
	}
	got, err := cache.GetQueryCache(ctx, "system:1")
	if err != nil {
		t.Fatalf("GetQueryCache() error = %v", err)
	}
	if got != "{\"ok\":true}" {
		t.Fatalf("GetQueryCache() = %q, want original payload", got)
	}
}

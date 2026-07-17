package statisticscache

import (
	"context"
	"strings"
	"testing"
	"time"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func statisticsPolicies(policy sharedcache.Policy) sharedcache.PolicyProvider {
	return sharedcache.NewRegistry(sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityStatisticsQuery, Policy: policy})
}

func TestStatisticsCacheUsesNamespacedQueryKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndProvider(client, keyspace.NewBuilderWithNamespace("stats-test"), statisticsPolicies(cachepolicy.CachePolicy{}))
	ctx := context.Background()

	if err := cache.SetQueryCache(ctx, "overview:1:preset:today:2026-01-01:2026-01-01", "{\"ok\":true}"); err != nil {
		t.Fatalf("set query cache failed: %v", err)
	}
	value, err := cache.GetQueryCache(ctx, "overview:1:preset:today:2026-01-01:2026-01-01")
	if err != nil {
		t.Fatalf("get query cache failed: %v", err)
	}
	if value != "{\"ok\":true}" {
		t.Fatalf("unexpected cache value: %s", value)
	}
	if !mr.Exists("stats-test:query:stats:query:overview:1:preset:today:2026-01-01:2026-01-01:v0") {
		t.Fatalf("expected versioned namespaced stats query key")
	}
	if mr.Exists("stats-test:query:version:stats:query:overview:1:preset:today:2026-01-01:2026-01-01") {
		t.Fatalf("unexpected version token key for static fallback store")
	}
}

func TestStatisticsCacheUsesExplicitBuilderNamespace(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndProvider(client, keyspace.NewBuilderWithNamespace("prod:cache:query"), statisticsPolicies(cachepolicy.CachePolicy{}))
	ctx := context.Background()

	if err := cache.SetQueryCache(ctx, "overview:1", "{\"ok\":true}"); err != nil {
		t.Fatalf("set query cache failed: %v", err)
	}
	if !mr.Exists("prod:cache:query:query:stats:query:overview:1:v0") {
		t.Fatalf("expected explicit namespaced versioned stats query key")
	}
}

func TestStatisticsCacheAppliesPolicyTTLAndCompression(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndProvider(
		client,
		keyspace.NewBuilderWithNamespace("prod:cache:query"),
		statisticsPolicies(cachepolicy.CachePolicy{
			TTL:      3 * time.Minute,
			Compress: cachepolicy.PolicySwitchEnabled,
		}),
	)
	ctx := context.Background()

	if err := cache.SetQueryCache(ctx, "overview:1", "{\"ok\":true}"); err != nil {
		t.Fatalf("set query cache failed: %v", err)
	}

	ttl := mr.TTL("prod:cache:query:query:stats:query:overview:1:v0")
	if ttl <= 0 || ttl > 3*time.Minute {
		t.Fatalf("expected policy ttl to be applied, got %v", ttl)
	}

	value, err := cache.GetQueryCache(ctx, "overview:1")
	if err != nil {
		t.Fatalf("get query cache failed: %v", err)
	}
	if value != "{\"ok\":true}" {
		t.Fatalf("unexpected cache value after compression roundtrip: %s", value)
	}
}

func TestStatisticsCacheReloadAffectsOnlyNewWritesAndKeepsGzipReadable(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	capability := sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityStatisticsQuery, Enabled: true,
		Policy: sharedcache.Policy{TTL: time.Minute, Compress: sharedcache.PolicySwitchEnabled}}
	registry := sharedcache.NewRegistry(capability)
	cache := NewStatisticsCacheWithBuilderAndProvider(client, keyspace.NewBuilderWithNamespace("reload"), registry)
	ctx := context.Background()
	if err := cache.SetQueryCache(ctx, "old", `{"value":"old"}`); err != nil {
		t.Fatal(err)
	}
	oldKey := "reload:query:stats:query:old:v0"
	oldTTL := mr.TTL(oldKey)

	capability.Policy = sharedcache.Policy{TTL: 5 * time.Minute, Compress: sharedcache.PolicySwitchDisabled}
	if _, err := registry.Publish(1, []sharedcache.EffectiveCapability{capability}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := cache.SetQueryCache(ctx, "new", `{"value":"new"}`); err != nil {
		t.Fatal(err)
	}
	newKey := "reload:query:stats:query:new:v0"
	if ttl := mr.TTL(newKey); ttl <= 4*time.Minute || ttl > 5*time.Minute {
		t.Fatalf("new TTL = %s, want approximately 5m", ttl)
	}
	if ttl := mr.TTL(oldKey); ttl != oldTTL {
		t.Fatalf("old TTL changed from %s to %s", oldTTL, ttl)
	}
	if got, err := mr.Get(newKey); err != nil || strings.HasPrefix(got, "\x1f\x8b") {
		t.Fatalf("new payload = %q, want uncompressed JSON", got)
	}
	if got, err := cache.GetQueryCache(ctx, "old"); err != nil || got != `{"value":"old"}` {
		t.Fatalf("old gzip payload after reload = %q, %v", got, err)
	}
}

func TestStatisticsCacheDegradesRedisReadErrorToMiss(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:63999"})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndProvider(client, keyspace.NewBuilderWithNamespace("prod:cache:query"), statisticsPolicies(cachepolicy.CachePolicy{}))
	value, err := cache.GetQueryCache(context.Background(), "overview:1")
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

	cache := NewStatisticsCacheWithBuilderProviderVersionStoreAndObserver(
		client,
		keyspace.NewBuilderWithNamespace("prod:cache:query"),
		statisticsPolicies(cachepolicy.CachePolicy{TTL: time.Minute}),
		nil,
		nil,
	)
	ctx := context.Background()

	if err := cache.SetQueryCache(ctx, "overview:1", "{\"ok\":true}"); err != nil {
		t.Fatalf("SetQueryCache() error = %v", err)
	}
	got, err := cache.GetQueryCache(ctx, "overview:1")
	if err != nil {
		t.Fatalf("GetQueryCache() error = %v", err)
	}
	if got != "{\"ok\":true}" {
		t.Fatalf("GetQueryCache() = %q, want original payload", got)
	}
}

func TestStatisticsTypedCacheDegradesInvalidJSONToMiss(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCacheWithBuilderAndProvider(client, keyspace.NewBuilderWithNamespace("stats-test"), statisticsPolicies(cachepolicy.CachePolicy{}))
	ctx := context.Background()

	timeRange := domainStatistics.StatisticsTimeRange{
		Preset: domainStatistics.TimeRangePresetToday,
		From:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local),
		To:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local),
	}
	if err := cache.SetQueryCache(ctx, overviewStatsCacheKey(12, timeRange), "{"); err != nil {
		t.Fatalf("SetQueryCache() error = %v", err)
	}
	if stats, ok := cache.LoadOverview(ctx, 12, timeRange); ok || stats != nil {
		t.Fatalf("LoadOverview() = (%+v, %v), want miss", stats, ok)
	}
}

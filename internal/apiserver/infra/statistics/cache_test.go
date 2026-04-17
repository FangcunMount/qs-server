package statistics

import (
	"context"
	"testing"
	"time"

	domainstats "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestStatisticsCacheUsesNamespacedKeys(t *testing.T) {
	rediskey.ApplyNamespace("stats-test")
	defer rediskey.ApplyNamespace("")

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCache(client)
	ctx := context.Background()
	now := time.Date(2026, 4, 15, 8, 0, 0, 0, time.Local)

	if err := cache.SetQueryCache(ctx, "system:1", "{\"ok\":true}", time.Minute); err != nil {
		t.Fatalf("set query cache failed: %v", err)
	}
	claimed, err := cache.TryMarkEventProcessed(ctx, "evt-1", now)
	if err != nil {
		t.Fatalf("try mark event processed failed: %v", err)
	}
	if !claimed {
		t.Fatalf("expected first event claim to succeed")
	}
	if err := cache.IncrementDailyCount(ctx, 1, domainstats.StatisticTypeQuestionnaire, "PHQ9", now, "submission"); err != nil {
		t.Fatalf("increment daily count failed: %v", err)
	}

	if !mr.Exists("stats-test:stats:query:system:1") {
		t.Fatalf("expected namespaced stats query key")
	}
	if !mr.Exists("stats-test:event:processed:bucket:2026-04-15") {
		t.Fatalf("expected namespaced event processed bucket key")
	}
	if !mr.Exists("stats-test:stats:daily:1:questionnaire:PHQ9:2026-04-15") {
		t.Fatalf("expected namespaced daily stats key")
	}
}

func TestTryMarkEventProcessedRejectsSameDayDuplicates(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCache(client)
	ctx := context.Background()
	now := time.Date(2026, 4, 17, 9, 30, 0, 0, time.Local)

	claimed, err := cache.TryMarkEventProcessed(ctx, "evt-1", now)
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if !claimed {
		t.Fatalf("expected first claim to succeed")
	}

	claimed, err = cache.TryMarkEventProcessed(ctx, "evt-1", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	if claimed {
		t.Fatalf("expected duplicate same-day claim to fail")
	}
}

func TestTryMarkEventProcessedRejectsLegacyKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCache(client)
	builder := rediskey.NewBuilder()
	ctx := context.Background()

	if err := client.Set(ctx, builder.BuildEventProcessedKey("evt-legacy"), "1", time.Hour).Err(); err != nil {
		t.Fatalf("seed legacy key failed: %v", err)
	}

	claimed, err := cache.TryMarkEventProcessed(ctx, "evt-legacy", time.Date(2026, 4, 17, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("claim with legacy key failed: %v", err)
	}
	if claimed {
		t.Fatalf("expected legacy key to block new claim")
	}
}

func TestTryMarkEventProcessedRejectsCrossDayDuplicatesWithinWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCache(client)
	ctx := context.Background()
	dayOne := time.Date(2026, 4, 17, 23, 30, 0, 0, time.Local)

	claimed, err := cache.TryMarkEventProcessed(ctx, "evt-cross-day", dayOne)
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if !claimed {
		t.Fatalf("expected first claim to succeed")
	}

	claimed, err = cache.TryMarkEventProcessed(ctx, "evt-cross-day", dayOne.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	if claimed {
		t.Fatalf("expected next-day duplicate within window to fail")
	}
}

func TestTryMarkEventProcessedSetsBucketTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewStatisticsCache(client)
	ctx := context.Background()
	now := time.Date(2026, 4, 17, 11, 0, 0, 0, time.Local)
	bucketKey := rediskey.NewBuilder().BuildEventProcessedBucketKey("2026-04-17")

	claimed, err := cache.TryMarkEventProcessed(ctx, "evt-ttl", now)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if !claimed {
		t.Fatalf("expected first claim to succeed")
	}

	ttl := mr.TTL(bucketKey)
	if ttl < (7*24*time.Hour) || ttl > (8*24*time.Hour) {
		t.Fatalf("expected bucket ttl near 8 days, got %s", ttl)
	}
}

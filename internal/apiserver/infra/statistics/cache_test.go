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

	if err := cache.SetQueryCache(ctx, "system:1", "{\"ok\":true}", time.Minute); err != nil {
		t.Fatalf("set query cache failed: %v", err)
	}
	if err := cache.MarkEventProcessed(ctx, "evt-1", time.Minute); err != nil {
		t.Fatalf("mark event processed failed: %v", err)
	}
	if err := cache.IncrementDailyCount(ctx, 1, domainstats.StatisticTypeQuestionnaire, "PHQ9", time.Date(2026, 4, 15, 8, 0, 0, 0, time.Local), "submission"); err != nil {
		t.Fatalf("increment daily count failed: %v", err)
	}

	if !mr.Exists("stats-test:stats:query:system:1") {
		t.Fatalf("expected namespaced stats query key")
	}
	if !mr.Exists("stats-test:event:processed:evt-1") {
		t.Fatalf("expected namespaced event processed key")
	}
	if !mr.Exists("stats-test:stats:daily:1:questionnaire:PHQ9:2026-04-15") {
		t.Fatalf("expected namespaced daily stats key")
	}
}

package statistics

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestStatisticsCacheUsesNamespacedQueryKeys(t *testing.T) {
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
	value, err := cache.GetQueryCache(ctx, "system:1")
	if err != nil {
		t.Fatalf("get query cache failed: %v", err)
	}
	if value != "{\"ok\":true}" {
		t.Fatalf("unexpected cache value: %s", value)
	}
	if !mr.Exists("stats-test:stats:query:system:1") {
		t.Fatalf("expected namespaced stats query key")
	}
}

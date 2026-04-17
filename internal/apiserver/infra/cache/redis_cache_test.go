package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisCacheDeletePattern(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewRedisCache(client)
	ctx := context.Background()
	for _, key := range []string{"stats:1", "stats:2", "keep:1"} {
		if err := client.Set(ctx, key, "1", 0).Err(); err != nil {
			t.Fatalf("seed key %s failed: %v", key, err)
		}
	}

	if err := cache.DeletePattern(ctx, "stats:*"); err != nil {
		t.Fatalf("DeletePattern() error = %v", err)
	}

	if mr.Exists("stats:1") || mr.Exists("stats:2") {
		t.Fatalf("expected stats keys to be deleted")
	}
	if !mr.Exists("keep:1") {
		t.Fatalf("expected non-matching key to remain")
	}
}

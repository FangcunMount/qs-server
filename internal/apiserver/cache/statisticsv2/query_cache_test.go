package statisticsv2

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestQueryCacheUsesGenerationAndFallsBackToL1Stale(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := NewQueryCache(client)
	type payload struct {
		Value int `json:"value"`
	}
	cache.Set(context.Background(), 7, "overview:7d", payload{Value: 42})
	var got payload
	if hit, stale := cache.Get(context.Background(), 7, "overview:7d", &got); !hit || stale || got.Value != 42 {
		t.Fatalf("hit=%v stale=%v got=%+v", hit, stale, got)
	}
	if _, err := cache.gen.Publish(context.Background(), 7, cache.now()); err != nil {
		t.Fatal(err)
	}
	if hit, _ := cache.Get(context.Background(), 7, "overview:7d", &got); hit {
		t.Fatal("old generation must not be a fresh hit")
	}
	cache.Set(context.Background(), 7, "overview:7d", payload{Value: 43})
	mr.Close()
	got = payload{}
	if hit, stale := cache.Get(context.Background(), 7, "overview:7d", &got); !hit || !stale || got.Value != 43 {
		t.Fatalf("hit=%v stale=%v got=%+v", hit, stale, got)
	}
}

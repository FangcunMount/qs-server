package cachequery

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestPublishedScaleHotListCacheStoresAndReadsPayload(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	cache := NewPublishedScaleHotListCacheWithPolicyAndKeyBuilder(
		cacheentry.NewRedisCache(client),
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
	)
	ctx := context.Background()
	payload := []byte(`{"items":[{"code":"3adyDE"}],"total":1,"limit":5,"window_days":30}`)

	if err := cache.Set(ctx, 5, 30, payload); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, ok := cache.Get(ctx, 5, 30)
	if !ok {
		t.Fatal("Get() cache miss")
	}
	if string(got) != string(payload) {
		t.Fatalf("Get() = %s, want %s", got, payload)
	}
}

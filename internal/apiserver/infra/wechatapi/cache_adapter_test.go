package wechatapi

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisCacheAdapterUsesExplicitBuilderNamespace(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cache := NewRedisCacheAdapterWithBuilder(client, rediskey.NewBuilderWithNamespace("prod:cache:sdk"))
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

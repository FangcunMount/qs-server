package main

import "testing"

func TestNewRedisClientUsesACLUsername(t *testing.T) {
	client := newRedisClient("127.0.0.1:6379", "stats-user", "secret", 3)
	defer func() {
		err := client.Close()
		if err != nil {
			t.Fatalf("failed to close redis client: %v", err)
		}
	}()

	opts := client.Options()
	if opts.Username != "stats-user" {
		t.Fatalf("expected redis username to be propagated, got %q", opts.Username)
	}
	if opts.Password != "secret" {
		t.Fatalf("expected redis password to be propagated, got %q", opts.Password)
	}
	if opts.DB != 3 {
		t.Fatalf("expected redis db to be propagated, got %d", opts.DB)
	}
}

func TestRebuildTargetDescriptionHonorsSkipCache(t *testing.T) {
	cfg := config{skipCache: true}

	if shouldRebuildCache(cfg) {
		t.Fatal("should not rebuild cache when --skip-cache is set")
	}
	if got := rebuildTargetDescription(cfg); got != "statistics aggregates" {
		t.Fatalf("target description = %q, want statistics aggregates", got)
	}
}

func TestRebuildTargetDescriptionIncludesRedisWhenConfigured(t *testing.T) {
	cfg := config{redisQueryAddr: "127.0.0.1:6379", redisMetaAddr: "127.0.0.1:6379"}

	if !shouldRebuildCache(cfg) {
		t.Fatal("should rebuild cache when Redis is configured and cache is not skipped")
	}
	if got := rebuildTargetDescription(cfg); got != "statistics aggregates and Redis query cache" {
		t.Fatalf("target description = %q, want aggregate and Redis targets", got)
	}
}

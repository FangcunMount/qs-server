package redisadapter

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestBackendAllowsThenLimitsWithRetryAfter(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	backend := NewBackend(client, keyspace.NewBuilderWithNamespace("ops:runtime"))

	allowed, retryAfter, err := backend.Allow(context.Background(), "limit:submit:global", 1, 1)
	if err != nil {
		t.Fatalf("first Allow() error = %v", err)
	}
	if !allowed || retryAfter != 0 {
		t.Fatalf("first Allow() = allowed %v retryAfter %s, want allowed with no retry", allowed, retryAfter)
	}

	allowed, retryAfter, err = backend.Allow(context.Background(), "limit:submit:global", 1, 1)
	if err != nil {
		t.Fatalf("second Allow() error = %v", err)
	}
	if allowed {
		t.Fatal("second Allow() should be limited")
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter = %s, want positive", retryAfter)
	}
}

func TestBackendRejectsInvalidInput(t *testing.T) {
	backend := NewBackend(nil, nil)
	if _, _, err := backend.Allow(context.Background(), "", 1, 1); err == nil {
		t.Fatal("expected empty key error")
	}
	if _, _, err := backend.Allow(context.Background(), "key", 0, 1); err == nil {
		t.Fatal("expected invalid rate error")
	}
	if _, _, err := backend.Allow(context.Background(), "key", 1, 0); err == nil {
		t.Fatal("expected invalid burst error")
	}
}

func TestBackendUnavailableReturnsErrorForCallerFallback(t *testing.T) {
	backend := NewBackend(nil, nil)
	if _, _, err := backend.Allow(context.Background(), "limit:submit:global", 1, 1); err == nil {
		t.Fatal("expected unavailable limiter error")
	}
}

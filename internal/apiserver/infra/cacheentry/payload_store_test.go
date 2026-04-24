package cacheentry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestPayloadStoreRoundTripsCompressedPayload(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	store := NewPayloadStore(NewRedisCache(client), cachepolicy.PolicyStatsQuery, cachepolicy.CachePolicy{
		Compress: cachepolicy.PolicySwitchEnabled,
	})
	ctx := context.Background()
	raw := []byte(`{"value":"compressed"}`)
	if err := store.Set(ctx, "payload:compressed", raw, time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	stored, err := NewRedisCache(client).Get(ctx, "payload:compressed")
	if err != nil {
		t.Fatalf("redis Get() error = %v", err)
	}
	if string(stored) == string(raw) {
		t.Fatal("stored payload should be compressed")
	}

	got, err := store.Get(ctx, "payload:compressed")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("Get() payload = %s, want %s", got, raw)
	}
}

func TestPayloadStoreNilCacheNoOpsAndPropagatesRedisErrors(t *testing.T) {
	t.Parallel()

	nilStore := NewPayloadStore(nil, cachepolicy.PolicyStatsQuery, cachepolicy.CachePolicy{})
	ctx := context.Background()
	if _, err := nilStore.Get(ctx, "payload:nil"); !errors.Is(err, ErrCacheNotFound) {
		t.Fatalf("nil Get() error = %v, want ErrCacheNotFound", err)
	}
	if err := nilStore.Set(ctx, "payload:nil", []byte("value"), time.Minute); err != nil {
		t.Fatalf("nil Set() error = %v", err)
	}
	if err := nilStore.SetNegative(ctx, "payload:nil", time.Minute); err != nil {
		t.Fatalf("nil SetNegative() error = %v", err)
	}
	if err := nilStore.Delete(ctx, "payload:nil"); err != nil {
		t.Fatalf("nil Delete() error = %v", err)
	}
	if exists, err := nilStore.Exists(ctx, "payload:nil"); err != nil || exists {
		t.Fatalf("nil Exists() = %v, %v; want false, nil", exists, err)
	}

	boom := errors.New("redis unavailable")
	errorStore := NewPayloadStore(errorCache{err: boom}, cachepolicy.PolicyStatsQuery, cachepolicy.CachePolicy{})
	if _, err := errorStore.Get(ctx, "payload:error"); !errors.Is(err, boom) {
		t.Fatalf("error Get() error = %v, want %v", err, boom)
	}
	if err := errorStore.Set(ctx, "payload:error", []byte("value"), time.Minute); !errors.Is(err, boom) {
		t.Fatalf("error Set() error = %v, want %v", err, boom)
	}
	if err := errorStore.Delete(ctx, "payload:error"); !errors.Is(err, boom) {
		t.Fatalf("error Delete() error = %v, want %v", err, boom)
	}
	if _, err := errorStore.Exists(ctx, "payload:error"); !errors.Is(err, boom) {
		t.Fatalf("error Exists() error = %v, want %v", err, boom)
	}
}

type errorCache struct {
	err error
}

func (c errorCache) Get(context.Context, string) ([]byte, error) {
	return nil, c.err
}

func (c errorCache) Set(context.Context, string, []byte, time.Duration) error {
	return c.err
}

func (c errorCache) Delete(context.Context, string) error {
	return c.err
}

func (c errorCache) Exists(context.Context, string) (bool, error) {
	return false, c.err
}

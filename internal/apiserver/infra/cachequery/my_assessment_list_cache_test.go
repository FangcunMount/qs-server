package cachequery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type assessmentListPayload struct {
	Total int `json:"total"`
}

func TestMyAssessmentListCacheUsesVersionTokenInvalidation(t *testing.T) {
	mr := miniredis.RunT(t)
	queryClient := redis.NewClient(&redis.Options{Addr: mr.Addr(), DB: 3})
	metaClient := redis.NewClient(&redis.Options{Addr: mr.Addr(), DB: 6})
	t.Cleanup(func() {
		_ = queryClient.Close()
		_ = metaClient.Close()
	})

	queryCache := cacheentry.NewRedisCache(queryClient)
	versionStore := NewRedisVersionTokenStore(metaClient)
	keyBuilder := rediskey.NewBuilderWithNamespace("cache:query")
	listCache := NewMyAssessmentListCacheWithBuilderAndPolicy(queryCache, versionStore, keyBuilder, cachepolicy.CachePolicy{
		TTL:         time.Minute,
		JitterRatio: 0,
	})

	ctx := context.Background()
	payload := &assessmentListPayload{Total: 1}

	listCache.Set(ctx, 42, 1, 10, "done", "SDS", "high", "", "", payload)

	version0Key := listCache.buildDataKey(42, 0, 1, 10, "done", "SDS", "high", "", "")
	if exists, err := queryClient.Exists(ctx, version0Key).Result(); err != nil || exists != 1 {
		t.Fatalf("expected v0 cache key %q to exist, exists=%d err=%v", version0Key, exists, err)
	}

	var cached assessmentListPayload
	if err := listCache.Get(ctx, 42, 1, 10, "done", "SDS", "high", "", "", &cached); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if cached.Total != 1 {
		t.Fatalf("Get() total = %d, want 1", cached.Total)
	}

	if err := listCache.Invalidate(ctx, 42); err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}
	if exists, err := queryClient.Exists(ctx, version0Key).Result(); err != nil || exists != 1 {
		t.Fatalf("expected old versioned key %q to remain until TTL, exists=%d err=%v", version0Key, exists, err)
	}

	gotVersion, err := metaClient.Get(ctx, listCache.buildVersionKey(42)).Result()
	if err != nil {
		t.Fatalf("read version token failed: %v", err)
	}
	if gotVersion != "1" {
		t.Fatalf("version token = %q, want 1", gotVersion)
	}

	var afterInvalidate assessmentListPayload
	if err := listCache.Get(ctx, 42, 1, 10, "done", "SDS", "high", "", "", &afterInvalidate); err != cacheentry.ErrCacheNotFound {
		t.Fatalf("Get() after invalidate error = %v, want %v", err, cacheentry.ErrCacheNotFound)
	}

	listCache.Set(ctx, 42, 1, 10, "done", "SDS", "high", "", "", &assessmentListPayload{Total: 2})
	version1Key := listCache.buildDataKey(42, 1, 1, 10, "done", "SDS", "high", "", "")
	if exists, err := queryClient.Exists(ctx, version1Key).Result(); err != nil || exists != 1 {
		t.Fatalf("expected v1 cache key %q to exist, exists=%d err=%v", version1Key, exists, err)
	}

	var refreshed assessmentListPayload
	if err := listCache.Get(ctx, 42, 1, 10, "done", "SDS", "high", "", "", &refreshed); err != nil {
		t.Fatalf("Get() after rewrite error = %v", err)
	}
	if refreshed.Total != 2 {
		t.Fatalf("Get() after rewrite total = %d, want 2", refreshed.Total)
	}
}

type failingVersionStore struct{}

func (f failingVersionStore) Current(context.Context, string) (uint64, error) {
	return 0, errors.New("meta unavailable")
}
func (f failingVersionStore) Bump(context.Context, string) (uint64, error) {
	return 0, errors.New("meta unavailable")
}

func TestMyAssessmentListCacheDegradesVersionReadFailureToMiss(t *testing.T) {
	mr := miniredis.RunT(t)
	queryClient := redis.NewClient(&redis.Options{Addr: mr.Addr(), DB: 3})
	t.Cleanup(func() {
		_ = queryClient.Close()
	})

	queryCache := cacheentry.NewRedisCache(queryClient)
	keyBuilder := rediskey.NewBuilderWithNamespace("cache:query")
	listCache := NewMyAssessmentListCacheWithBuilderAndPolicy(queryCache, failingVersionStore{}, keyBuilder, cachepolicy.CachePolicy{
		TTL:         time.Minute,
		JitterRatio: 0,
	})

	var cached assessmentListPayload
	if err := listCache.Get(context.Background(), 42, 1, 10, "done", "SDS", "high", "", "", &cached); err != cacheentry.ErrCacheNotFound {
		t.Fatalf("Get() error = %v, want %v", err, cacheentry.ErrCacheNotFound)
	}
}

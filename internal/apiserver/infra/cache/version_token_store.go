package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	redis "github.com/redis/go-redis/v9"
)

// VersionTokenStore 维护 query/list cache 的 version token。
type VersionTokenStore interface {
	Current(ctx context.Context, versionKey string) (uint64, error)
	Bump(ctx context.Context, versionKey string) (uint64, error)
}

// RedisVersionTokenStore 使用 Redis 存储 version token。
type RedisVersionTokenStore struct {
	client redis.UniversalClient
	kind   string
}

func NewRedisVersionTokenStore(client redis.UniversalClient) VersionTokenStore {
	return NewRedisVersionTokenStoreWithKind(client, "unknown")
}

func NewRedisVersionTokenStoreWithKind(client redis.UniversalClient, kind string) VersionTokenStore {
	if client == nil {
		return nil
	}
	if kind == "" {
		kind = "unknown"
	}
	return &RedisVersionTokenStore{client: client, kind: kind}
}

func (s *RedisVersionTokenStore) Current(ctx context.Context, versionKey string) (uint64, error) {
	if s == nil || s.client == nil {
		return 0, fmt.Errorf("version token redis client is nil")
	}

	start := time.Now()
	value, err := s.client.Get(ctx, versionKey).Result()
	if err == redis.Nil {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
		cacheobservability.ObserveFamilySuccess("apiserver", "meta_hotset")
		return 0, nil
	}
	if err != nil {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "error", time.Since(start))
		cacheobservability.ObserveFamilyFailure("apiserver", "meta_hotset", err)
		return 0, err
	}
	if value == "" {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
		cacheobservability.ObserveFamilySuccess("apiserver", "meta_hotset")
		return 0, nil
	}

	token, parseErr := strconv.ParseUint(value, 10, 64)
	if parseErr != nil {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "error", time.Since(start))
		cacheobservability.ObserveFamilyFailure("apiserver", "meta_hotset", parseErr)
		return 0, fmt.Errorf("parse version token %q: %w", versionKey, parseErr)
	}
	cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
	cacheobservability.ObserveFamilySuccess("apiserver", "meta_hotset")
	return token, nil
}

func (s *RedisVersionTokenStore) Bump(ctx context.Context, versionKey string) (uint64, error) {
	if s == nil || s.client == nil {
		return 0, fmt.Errorf("version token redis client is nil")
	}
	start := time.Now()
	token, err := s.client.Incr(ctx, versionKey).Uint64()
	if err != nil {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "bump", "error", time.Since(start))
		cacheobservability.ObserveFamilyFailure("apiserver", "meta_hotset", err)
		return 0, err
	}
	cacheobservability.ObserveQueryCacheVersion(s.kind, "bump", "ok", time.Since(start))
	cacheobservability.ObserveFamilySuccess("apiserver", "meta_hotset")
	return token, nil
}

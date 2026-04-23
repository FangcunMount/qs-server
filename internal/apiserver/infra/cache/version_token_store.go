package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	redis "github.com/redis/go-redis/v9"
)

// VersionTokenStore 维护 query/list cache 的 version token。
type VersionTokenStore interface {
	Current(ctx context.Context, versionKey string) (uint64, error)
	Bump(ctx context.Context, versionKey string) (uint64, error)
}

// RedisVersionTokenStore 使用 Redis 存储 version token。
type RedisVersionTokenStore struct {
	client   redis.UniversalClient
	kind     string
	observer *Observer
}

func NewRedisVersionTokenStore(client redis.UniversalClient) VersionTokenStore {
	return NewRedisVersionTokenStoreWithKind(client, "unknown")
}

func NewRedisVersionTokenStoreWithKind(client redis.UniversalClient, kind string) VersionTokenStore {
	return NewRedisVersionTokenStoreWithKindAndObserver(client, kind, nil)
}

func NewRedisVersionTokenStoreWithKindAndObserver(client redis.UniversalClient, kind string, observer *Observer) VersionTokenStore {
	if client == nil {
		return nil
	}
	if kind == "" {
		kind = "unknown"
	}
	return &RedisVersionTokenStore{client: client, kind: kind, observer: observer}
}

func (s *RedisVersionTokenStore) Current(ctx context.Context, versionKey string) (uint64, error) {
	if s == nil || s.client == nil {
		return 0, fmt.Errorf("version token redis client is nil")
	}

	start := time.Now()
	value, err := s.client.Get(ctx, versionKey).Result()
	if err == redis.Nil {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
		s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
		return 0, nil
	}
	if err != nil {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "error", time.Since(start))
		s.observer.ObserveFamilyFailure(string(redisplane.FamilyMeta), err)
		return 0, err
	}
	if value == "" {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
		s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
		return 0, nil
	}

	token, parseErr := strconv.ParseUint(value, 10, 64)
	if parseErr != nil {
		cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "error", time.Since(start))
		s.observer.ObserveFamilyFailure(string(redisplane.FamilyMeta), parseErr)
		return 0, fmt.Errorf("parse version token %q: %w", versionKey, parseErr)
	}
	cacheobservability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
	s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
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
		s.observer.ObserveFamilyFailure(string(redisplane.FamilyMeta), err)
		return 0, err
	}
	cacheobservability.ObserveQueryCacheVersion(s.kind, "bump", "ok", time.Since(start))
	s.observer.ObserveFamilySuccess(string(redisplane.FamilyMeta))
	return token, nil
}

type staticVersionTokenStore struct {
	version uint64
}

func NewStaticVersionTokenStore(version uint64) VersionTokenStore {
	return staticVersionTokenStore{version: version}
}

func (s staticVersionTokenStore) Current(context.Context, string) (uint64, error) {
	return s.version, nil
}

func (s staticVersionTokenStore) Bump(context.Context, string) (uint64, error) {
	return s.version, nil
}

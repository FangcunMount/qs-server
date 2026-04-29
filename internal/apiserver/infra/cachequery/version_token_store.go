package cachequery

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	redis "github.com/redis/go-redis/v9"
)

// VersionTokenStore maintains query/list cache version tokens.
type VersionTokenStore interface {
	Current(ctx context.Context, versionKey string) (uint64, error)
	Bump(ctx context.Context, versionKey string) (uint64, error)
}

// RedisVersionTokenStore stores version tokens in Redis.
type RedisVersionTokenStore struct {
	client   redis.UniversalClient
	kind     string
	observer FamilyObserver
}

func NewRedisVersionTokenStore(client redis.UniversalClient) VersionTokenStore {
	return NewRedisVersionTokenStoreWithKind(client, "unknown")
}

func NewRedisVersionTokenStoreWithKind(client redis.UniversalClient, kind string) VersionTokenStore {
	return NewRedisVersionTokenStoreWithKindAndObserver(client, kind, nil)
}

func NewRedisVersionTokenStoreWithKindAndObserver(client redis.UniversalClient, kind string, observer FamilyObserver) VersionTokenStore {
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
		observability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
		s.observeSuccess(string(cacheplane.FamilyMeta))
		return 0, nil
	}
	if err != nil {
		observability.ObserveQueryCacheVersion(s.kind, "current", "error", time.Since(start))
		s.observeFailure(string(cacheplane.FamilyMeta), err)
		return 0, err
	}
	if value == "" {
		observability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
		s.observeSuccess(string(cacheplane.FamilyMeta))
		return 0, nil
	}

	token, parseErr := strconv.ParseUint(value, 10, 64)
	if parseErr != nil {
		observability.ObserveQueryCacheVersion(s.kind, "current", "error", time.Since(start))
		s.observeFailure(string(cacheplane.FamilyMeta), parseErr)
		return 0, fmt.Errorf("parse version token %q: %w", versionKey, parseErr)
	}
	observability.ObserveQueryCacheVersion(s.kind, "current", "ok", time.Since(start))
	s.observeSuccess(string(cacheplane.FamilyMeta))
	return token, nil
}

func (s *RedisVersionTokenStore) Bump(ctx context.Context, versionKey string) (uint64, error) {
	if s == nil || s.client == nil {
		return 0, fmt.Errorf("version token redis client is nil")
	}
	start := time.Now()
	token, err := s.client.Incr(ctx, versionKey).Uint64()
	if err != nil {
		observability.ObserveQueryCacheVersion(s.kind, "bump", "error", time.Since(start))
		s.observeFailure(string(cacheplane.FamilyMeta), err)
		return 0, err
	}
	observability.ObserveQueryCacheVersion(s.kind, "bump", "ok", time.Since(start))
	s.observeSuccess(string(cacheplane.FamilyMeta))
	return token, nil
}

func (s *RedisVersionTokenStore) observeSuccess(family string) {
	if s != nil && s.observer != nil {
		s.observer.ObserveFamilySuccess(family)
	}
}

func (s *RedisVersionTokenStore) observeFailure(family string, err error) {
	if s != nil && s.observer != nil {
		s.observer.ObserveFamilyFailure(family, err)
	}
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

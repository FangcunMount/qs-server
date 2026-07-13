package query

import (
	"context"
	"fmt"
	"strconv"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type VersionTokenStore interface {
	Current(ctx context.Context, versionKey string) (uint64, error)
	Bump(ctx context.Context, versionKey string) (uint64, error)
}

type RedisVersionTokenStore struct {
	client   redis.UniversalClient
	observer VersionObserver
}

func NewRedisVersionTokenStore(client redis.UniversalClient, observer VersionObserver) VersionTokenStore {
	if client == nil {
		return nil
	}
	return &RedisVersionTokenStore{client: client, observer: observer}
}

func (s *RedisVersionTokenStore) Current(ctx context.Context, versionKey string) (uint64, error) {
	if s == nil || s.client == nil {
		return 0, fmt.Errorf("version token redis client is nil")
	}
	start := time.Now()
	value, err := s.client.Get(ctx, versionKey).Result()
	if err == redis.Nil || (err == nil && value == "") {
		s.observe("current", "ok", time.Since(start), nil)
		return 0, nil
	}
	if err != nil {
		s.observe("current", "error", time.Since(start), err)
		return 0, err
	}
	token, parseErr := strconv.ParseUint(value, 10, 64)
	if parseErr != nil {
		s.observe("current", "error", time.Since(start), parseErr)
		return 0, fmt.Errorf("parse version token %q: %w", versionKey, parseErr)
	}
	s.observe("current", "ok", time.Since(start), nil)
	return token, nil
}

func (s *RedisVersionTokenStore) Bump(ctx context.Context, versionKey string) (uint64, error) {
	if s == nil || s.client == nil {
		return 0, fmt.Errorf("version token redis client is nil")
	}
	start := time.Now()
	token, err := s.client.Incr(ctx, versionKey).Uint64()
	if err != nil {
		s.observe("bump", "error", time.Since(start), err)
		return 0, err
	}
	s.observe("bump", "ok", time.Since(start), nil)
	return token, nil
}

func (s *RedisVersionTokenStore) observe(operation, result string, duration time.Duration, err error) {
	if s == nil || s.observer == nil {
		return
	}
	s.observer.ObserveVersion(operation, result, duration)
	if err != nil {
		s.observer.ObserveFailure(err)
		return
	}
	s.observer.ObserveSuccess()
}

type staticVersionTokenStore struct{ version uint64 }

func NewStaticVersionTokenStore(version uint64) VersionTokenStore {
	return staticVersionTokenStore{version: version}
}

func (s staticVersionTokenStore) Current(context.Context, string) (uint64, error) {
	return s.version, nil
}
func (s staticVersionTokenStore) Bump(context.Context, string) (uint64, error) { return s.version, nil }

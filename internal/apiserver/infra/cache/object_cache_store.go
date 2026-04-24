package cache

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	redis "github.com/redis/go-redis/v9"
)

// ObjectCacheStore owns Redis entry storage details for object repository caches.
type ObjectCacheStore[T any] struct {
	policyKey   cachepolicy.CachePolicyKey
	policy      cachepolicy.CachePolicy
	ttl         time.Duration
	negativeTTL time.Duration
	codec       CacheEntryCodec[T]
	payload     *cacheentry.PayloadStore
}

type ObjectCacheStoreOptions[T any] struct {
	Cache       cacheentry.Cache
	PolicyKey   cachepolicy.CachePolicyKey
	Policy      cachepolicy.CachePolicy
	TTL         time.Duration
	NegativeTTL time.Duration
	Codec       CacheEntryCodec[T]
}

func NewObjectCacheStore[T any](opts ObjectCacheStoreOptions[T]) *ObjectCacheStore[T] {
	return &ObjectCacheStore[T]{
		policyKey:   opts.PolicyKey,
		policy:      opts.Policy,
		ttl:         opts.TTL,
		negativeTTL: opts.NegativeTTL,
		codec:       opts.Codec,
		payload:     cacheentry.NewPayloadStore(opts.Cache, opts.PolicyKey, opts.Policy),
	}
}

func (s *ObjectCacheStore[T]) Get(ctx context.Context, key string) (*T, error) {
	if s == nil || s.payload == nil {
		return nil, cacheentry.ErrCacheNotFound
	}

	data, err := s.payload.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	return s.codec.Decode(data)
}

func (s *ObjectCacheStore[T]) Set(ctx context.Context, key string, value *T) error {
	if s == nil {
		return nil
	}
	return s.SetWithTTL(ctx, key, value, s.ttl)
}

func (s *ObjectCacheStore[T]) SetWithTTL(ctx context.Context, key string, value *T, ttl time.Duration) error {
	if s == nil || s.payload == nil || value == nil {
		return nil
	}

	data, err := s.codec.Encode(value)
	if err != nil {
		return err
	}
	return s.payload.Set(ctx, key, data, ttl)
}

func (s *ObjectCacheStore[T]) SetNegative(ctx context.Context, key string) error {
	if s == nil || s.payload == nil {
		return nil
	}

	ttl := s.policy.NegativeTTLOr(s.negativeTTL)
	return s.payload.SetNegative(ctx, key, ttl)
}

func (s *ObjectCacheStore[T]) Delete(ctx context.Context, key string) error {
	if s == nil || s.payload == nil {
		return nil
	}
	return s.payload.Delete(ctx, key)
}

func (s *ObjectCacheStore[T]) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil || s.payload == nil {
		return false, nil
	}
	return s.payload.Exists(ctx, key)
}

func (s *ObjectCacheStore[T]) available() bool {
	return s != nil && s.payload != nil && s.payload.Available()
}

func newRedisCacheIfAvailable(client redis.UniversalClient) cacheentry.Cache {
	if client == nil {
		return nil
	}
	return cacheentry.NewRedisCache(client)
}

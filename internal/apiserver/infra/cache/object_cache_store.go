package cache

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
)

// ObjectCacheStore owns Redis entry storage details for object repository caches.
type ObjectCacheStore[T any] struct {
	cache       Cache
	policyKey   cachepolicy.CachePolicyKey
	policy      cachepolicy.CachePolicy
	ttl         time.Duration
	negativeTTL time.Duration
	codec       CacheEntryCodec[T]
}

type ObjectCacheStoreOptions[T any] struct {
	Cache       Cache
	PolicyKey   cachepolicy.CachePolicyKey
	Policy      cachepolicy.CachePolicy
	TTL         time.Duration
	NegativeTTL time.Duration
	Codec       CacheEntryCodec[T]
}

func NewObjectCacheStore[T any](opts ObjectCacheStoreOptions[T]) *ObjectCacheStore[T] {
	return &ObjectCacheStore[T]{
		cache:       opts.Cache,
		policyKey:   opts.PolicyKey,
		policy:      opts.Policy,
		ttl:         opts.TTL,
		negativeTTL: opts.NegativeTTL,
		codec:       opts.Codec,
	}
}

func (s *ObjectCacheStore[T]) Get(ctx context.Context, key string) (*T, error) {
	if s == nil || s.cache == nil {
		return nil, ErrCacheNotFound
	}

	cachedData, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(cachedData) == 0 {
		return nil, nil
	}

	data := s.policy.DecompressValue(cachedData)
	observePayload(s.policyKey, len(data), len(cachedData))
	return s.codec.Decode(data)
}

func (s *ObjectCacheStore[T]) Set(ctx context.Context, key string, value *T) error {
	if s == nil {
		return nil
	}
	return s.SetWithTTL(ctx, key, value, s.ttl)
}

func (s *ObjectCacheStore[T]) SetWithTTL(ctx context.Context, key string, value *T, ttl time.Duration) error {
	if s == nil || s.cache == nil || value == nil {
		return nil
	}

	data, err := s.codec.Encode(value)
	if err != nil {
		return err
	}
	payload := s.policy.CompressValue(data)
	observePayload(s.policyKey, len(data), len(payload))
	return s.cache.Set(ctx, key, payload, s.policy.JitterTTL(ttl))
}

func (s *ObjectCacheStore[T]) SetNegative(ctx context.Context, key string) error {
	if s == nil || s.cache == nil {
		return nil
	}

	ttl := s.policy.NegativeTTLOr(s.negativeTTL)
	return s.cache.Set(ctx, key, []byte{}, s.policy.JitterTTL(ttl))
}

func (s *ObjectCacheStore[T]) Delete(ctx context.Context, key string) error {
	if s == nil || s.cache == nil {
		return nil
	}

	if err := s.cache.Delete(ctx, key); err != nil {
		observeInvalidate(s.policyKey, "error")
		return err
	}
	observeInvalidate(s.policyKey, "ok")
	return nil
}

func (s *ObjectCacheStore[T]) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil || s.cache == nil {
		return false, nil
	}
	return s.cache.Exists(ctx, key)
}

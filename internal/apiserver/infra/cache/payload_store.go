package cache

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
)

// cachePayloadStore owns Redis payload-level behavior shared by object and query caches.
type cachePayloadStore struct {
	cache     Cache
	policyKey cachepolicy.CachePolicyKey
	policy    cachepolicy.CachePolicy
}

func newCachePayloadStore(cache Cache, policyKey cachepolicy.CachePolicyKey, policy cachepolicy.CachePolicy) *cachePayloadStore {
	return &cachePayloadStore{
		cache:     cache,
		policyKey: policyKey,
		policy:    policy,
	}
}

func (s *cachePayloadStore) Get(ctx context.Context, key string) ([]byte, error) {
	if s == nil || s.cache == nil {
		return nil, ErrCacheNotFound
	}
	payload, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, nil
	}
	raw := s.policy.DecompressValue(payload)
	observePayload(s.policyKey, len(raw), len(payload))
	return raw, nil
}

func (s *cachePayloadStore) Set(ctx context.Context, key string, raw []byte, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	payload := s.policy.CompressValue(raw)
	observePayload(s.policyKey, len(raw), len(payload))
	return s.cache.Set(ctx, key, payload, s.policy.JitterTTL(ttl))
}

func (s *cachePayloadStore) SetNegative(ctx context.Context, key string, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.Set(ctx, key, []byte{}, s.policy.JitterTTL(ttl))
}

func (s *cachePayloadStore) Delete(ctx context.Context, key string) error {
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

func (s *cachePayloadStore) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil || s.cache == nil {
		return false, nil
	}
	return s.cache.Exists(ctx, key)
}

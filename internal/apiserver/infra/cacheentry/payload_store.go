package cacheentry

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
)

// PayloadStore owns Redis payload-level behavior shared by object and query caches.
type PayloadStore struct {
	cache     Cache
	policyKey cachepolicy.CachePolicyKey
	policy    cachepolicy.CachePolicy
}

func NewPayloadStore(cache Cache, policyKey cachepolicy.CachePolicyKey, policy cachepolicy.CachePolicy) *PayloadStore {
	return &PayloadStore{
		cache:     cache,
		policyKey: policyKey,
		policy:    policy,
	}
}

func (s *PayloadStore) Get(ctx context.Context, key string) ([]byte, error) {
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
	ObservePayload(s.policyKey, len(raw), len(payload))
	return raw, nil
}

func (s *PayloadStore) Set(ctx context.Context, key string, raw []byte, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	payload := s.policy.CompressValue(raw)
	ObservePayload(s.policyKey, len(raw), len(payload))
	return s.cache.Set(ctx, key, payload, s.policy.JitterTTL(ttl))
}

func (s *PayloadStore) SetNegative(ctx context.Context, key string, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.Set(ctx, key, []byte{}, s.policy.JitterTTL(ttl))
}

func (s *PayloadStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.cache == nil {
		return nil
	}
	if err := s.cache.Delete(ctx, key); err != nil {
		ObserveInvalidate(s.policyKey, "error")
		return err
	}
	ObserveInvalidate(s.policyKey, "ok")
	return nil
}

func (s *PayloadStore) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil || s.cache == nil {
		return false, nil
	}
	return s.cache.Exists(ctx, key)
}

func (s *PayloadStore) Available() bool {
	return s != nil && s.cache != nil
}

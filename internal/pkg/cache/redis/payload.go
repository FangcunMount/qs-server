package redisstore

import (
	"context"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

// PayloadStore owns compression, TTL jitter and negative-sentinel bytes.
type PayloadStore struct {
	store    sharedcache.Store
	observer sharedcache.Observer
}

func NewPayloadStore(store sharedcache.Store, observer sharedcache.Observer) *PayloadStore {
	return &PayloadStore{store: store, observer: observer}
}

func (s *PayloadStore) Get(ctx context.Context, key string) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, sharedcache.ErrMiss
	}
	payload, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, nil
	}
	raw := sharedcache.DecompressData(payload)
	sharedcache.Observe(s.observer, sharedcache.Event{Operation: sharedcache.OperationPayloadRaw, Size: len(raw)})
	sharedcache.Observe(s.observer, sharedcache.Event{Operation: sharedcache.OperationPayloadSet, Size: len(payload)})
	return raw, nil
}

func (s *PayloadStore) Set(ctx context.Context, key string, raw []byte, ttl time.Duration, policy sharedcache.Policy) error {
	if s == nil || s.store == nil {
		return nil
	}
	payload := policy.CompressValue(raw)
	sharedcache.Observe(s.observer, sharedcache.Event{Operation: sharedcache.OperationPayloadRaw, Size: len(raw)})
	sharedcache.Observe(s.observer, sharedcache.Event{Operation: sharedcache.OperationPayloadSet, Size: len(payload)})
	return s.store.Set(ctx, key, payload, policy.JitterTTL(ttl))
}

func (s *PayloadStore) SetNegative(ctx context.Context, key string, ttl time.Duration, policy sharedcache.Policy) error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Set(ctx, key, []byte{}, policy.JitterTTL(ttl))
}

func (s *PayloadStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.store == nil {
		return nil
	}
	err := s.store.Delete(ctx, key)
	result := sharedcache.ResultOK
	if err != nil {
		result = sharedcache.ResultError
	}
	sharedcache.Observe(s.observer, sharedcache.Event{Operation: sharedcache.OperationInvalidate, Result: result, Err: err})
	return err
}

func (s *PayloadStore) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil || s.store == nil {
		return false, nil
	}
	return s.store.Exists(ctx, key)
}

func (s *PayloadStore) Available() bool {
	return s != nil && s.store != nil
}

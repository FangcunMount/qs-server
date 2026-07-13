package object

import (
	"context"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

// Store owns typed object encoding above the shared Redis payload store.
type Store[T any] struct {
	codec     Codec[T]
	payload   *redisstore.PayloadStore
	coalescer loadguard.Coalescer
}

type StoreOptions[T any] struct {
	Store     sharedcache.Store
	Codec     Codec[T]
	Observer  sharedcache.Observer
	Coalescer loadguard.Coalescer
}

func NewStore[T any](opts StoreOptions[T]) *Store[T] {
	return &Store[T]{
		codec: opts.Codec, payload: redisstore.NewPayloadStore(opts.Store, opts.Observer),
		coalescer: opts.Coalescer,
	}
}

func (s *Store[T]) Coalescer() loadguard.Coalescer {
	if s == nil {
		return nil
	}
	return s.coalescer
}

func (s *Store[T]) Get(ctx context.Context, key string) (*T, error) {
	if s == nil || s.payload == nil {
		return nil, sharedcache.ErrMiss
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

func (s *Store[T]) Set(ctx context.Context, key string, value *T, policy sharedcache.Policy) error {
	if s == nil || s.payload == nil || value == nil {
		return nil
	}
	data, err := s.codec.Encode(value)
	if err != nil {
		return err
	}
	return s.payload.Set(ctx, key, data, policy.TTL, policy)
}

func (s *Store[T]) SetNegative(ctx context.Context, key string, policy sharedcache.Policy) error {
	if s == nil || s.payload == nil {
		return nil
	}
	return s.payload.SetNegative(ctx, key, policy.NegativeTTL, policy)
}

func (s *Store[T]) Delete(ctx context.Context, key string) error {
	if s == nil || s.payload == nil {
		return nil
	}
	return s.payload.Delete(ctx, key)
}

func (s *Store[T]) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil || s.payload == nil {
		return false, nil
	}
	return s.payload.Exists(ctx, key)
}

func (s *Store[T]) Available() bool {
	return s != nil && s.payload != nil && s.payload.Available()
}

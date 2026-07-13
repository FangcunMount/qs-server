package object

import (
	"context"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

// Store owns typed object encoding above the shared Redis payload store.
type Store[T any] struct {
	policy      sharedcache.Policy
	ttl         time.Duration
	negativeTTL time.Duration
	codec       Codec[T]
	payload     *redisstore.PayloadStore
	coalescer   loadguard.Coalescer
}

type StoreOptions[T any] struct {
	Store       sharedcache.Store
	Policy      sharedcache.Policy
	TTL         time.Duration
	NegativeTTL time.Duration
	Codec       Codec[T]
	Observer    sharedcache.Observer
	Coalescer   loadguard.Coalescer
}

func NewStore[T any](opts StoreOptions[T]) *Store[T] {
	return &Store[T]{
		policy:      opts.Policy,
		ttl:         opts.TTL,
		negativeTTL: opts.NegativeTTL,
		codec:       opts.Codec,
		payload:     redisstore.NewPayloadStore(opts.Store, opts.Policy, opts.Observer),
		coalescer:   opts.Coalescer,
	}
}

func (s *Store[T]) Coalescer() loadguard.Coalescer {
	if s == nil {
		return nil
	}
	return s.coalescer
}

func (s *Store[T]) SetTTL(ttl time.Duration) {
	if s != nil {
		s.ttl = ttl
	}
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

func (s *Store[T]) Set(ctx context.Context, key string, value *T) error {
	if s == nil {
		return nil
	}
	return s.SetWithTTL(ctx, key, value, s.ttl)
}

func (s *Store[T]) SetWithTTL(ctx context.Context, key string, value *T, ttl time.Duration) error {
	if s == nil || s.payload == nil || value == nil {
		return nil
	}
	data, err := s.codec.Encode(value)
	if err != nil {
		return err
	}
	return s.payload.Set(ctx, key, data, ttl)
}

func (s *Store[T]) SetNegative(ctx context.Context, key string) error {
	if s == nil || s.payload == nil {
		return nil
	}
	return s.payload.SetNegative(ctx, key, s.policy.NegativeTTLOr(s.negativeTTL))
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

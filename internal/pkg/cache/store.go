package cache

import (
	"context"
	"errors"
	"time"
)

// ErrMiss means the requested cache entry does not exist.
var ErrMiss = errors.New("cache not found")

// Store is the minimum payload store required by current object/query caches.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

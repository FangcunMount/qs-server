package eventoutbox

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
)

func WithLimiter(limiter backpressure.Acquirer) StoreOption {
	return func(s *Store) {
		s.limiter = limiter
	}
}

func (s *Store) acquire(ctx context.Context) (context.Context, func(), error) {
	if s == nil || s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}

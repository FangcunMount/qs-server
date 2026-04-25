package mongo

import (
	"context"
	"errors"
	"testing"
)

func TestBaseRepositoryUsesInjectedLimiter(t *testing.T) {
	wantErr := errors.New("backpressure timeout")
	repo := BaseRepository{
		limiter: failingAcquirer{err: wantErr},
	}

	if _, err := repo.CountDocuments(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Fatalf("CountDocuments() error = %v, want %v", err, wantErr)
	}
}

type failingAcquirer struct {
	err error
}

func (f failingAcquirer) Acquire(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, f.err
}

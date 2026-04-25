package mysql

import (
	"context"
	"errors"
	"testing"
)

func TestBaseRepositoryUsesInjectedLimiter(t *testing.T) {
	wantErr := errors.New("backpressure timeout")
	repo := NewBaseRepositoryWithOptions[*testSyncable](nil, BaseRepositoryOptions{
		Limiter: failingAcquirer{err: wantErr},
	})

	if _, err := repo.FindByID(context.Background(), 1); !errors.Is(err, wantErr) {
		t.Fatalf("FindByID() error = %v, want %v", err, wantErr)
	}
}

type testSyncable struct {
	AuditFields
}

type failingAcquirer struct {
	err error
}

func (f failingAcquirer) Acquire(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, f.err
}

package iam

import (
	"context"
	"errors"
	"testing"
)

func TestIdentityServiceUsesInjectedLimiter(t *testing.T) {
	wantErr := errors.New("backpressure timeout")
	service := &IdentityService{
		enabled: true,
		limiter: failingAcquirer{err: wantErr},
	}

	if _, err := service.GetUser(context.Background(), "user-1"); !errors.Is(err, wantErr) {
		t.Fatalf("GetUser() error = %v, want %v", err, wantErr)
	}
}

type failingAcquirer struct {
	err error
}

func (f failingAcquirer) Acquire(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, f.err
}

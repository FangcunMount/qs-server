package aibridge

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServeRelayRetriesDatabaseFailureAndStopsWithoutAnotherBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	serveRelay(ctx, func(batch context.Context) (int, error) {
		calls++
		deadline, ok := batch.Deadline()
		if !ok || time.Until(deadline) > 60*time.Second {
			t.Fatal("batch requires a bounded deadline")
		}
		if calls == 1 {
			return 0, errors.New("database unavailable")
		}
		cancel()
		return 1, nil
	}, time.Millisecond, 4*time.Millisecond)
	if calls != 2 {
		t.Fatalf("batches = %d", calls)
	}
}

func TestServeRelayCancellationInterruptsIdleWait(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	called, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		serveRelay(ctx, func(context.Context) (int, error) { close(called); return 0, nil }, time.Hour, time.Hour)
	}()
	<-called
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("relay did not stop")
	}
}

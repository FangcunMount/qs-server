package admission

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitStrategyTimesOut(t *testing.T) {
	sem := NewChannelSemaphore(1)
	if !sem.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}

	strategy := WithWaitObserver(WaitStrategy{Sem: sem, MaxWait: 10 * time.Millisecond}, func(time.Duration) {})
	release, _, err := strategy.Acquire(t.Context())
	if !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("Acquire() error = %v, want ErrWaitTimeout", err)
	}
	if release != nil {
		t.Fatal("expected nil release on timeout")
	}
}

func TestWaitStrategyRespectsContextCancel(t *testing.T) {
	sem := NewChannelSemaphore(1)
	if !sem.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	strategy := WaitStrategy{Sem: sem, MaxWait: time.Second}
	_, _, err := strategy.Acquire(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire() error = %v, want context.Canceled", err)
	}
}

func TestTryStrategyRejectsWhenFull(t *testing.T) {
	sem := NewChannelSemaphore(1)
	if !sem.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}

	strategy := TryStrategy{Sem: sem}
	_, _, err := strategy.Acquire(t.Context())
	if !errors.Is(err, ErrTryRejected) {
		t.Fatalf("Acquire() error = %v, want ErrTryRejected", err)
	}
}

func TestBlockingStrategyAcquiresAfterRelease(t *testing.T) {
	sem := NewChannelSemaphore(1)
	if !sem.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}

	done := make(chan error, 1)
	go func() {
		strategy := BlockingStrategy{Sem: sem}
		release, _, err := strategy.Acquire(t.Context())
		if err != nil {
			done <- err
			return
		}
		defer release()
		done <- nil
	}()

	time.Sleep(20 * time.Millisecond)
	sem.Release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocking acquire did not complete after release")
	}
}

func TestNoopSemaphoreAlwaysAcquires(t *testing.T) {
	sem := NewChannelSemaphore(0)
	strategy := TryStrategy{Sem: sem}
	release, _, err := strategy.Acquire(t.Context())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	release()
}

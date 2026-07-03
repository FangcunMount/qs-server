package loadguard_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

func TestSingleflightCoalescerCoalescesConcurrentMiss(t *testing.T) {
	var loads atomic.Int32
	coalescer := loadguard.NewCoalescer(true)
	const workers = 8
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := coalescer.Do(context.Background(), "k", func() (any, error) {
				loads.Add(1)
				time.Sleep(50 * time.Millisecond)
				return "v", nil
			})
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
}

func TestSingleflightCoalescerForgetNilMissAllowsRetry(t *testing.T) {
	t.Parallel()

	var loads atomic.Int32
	coalescer := loadguard.NewCoalescer(true)
	load := func() (any, error) {
		loads.Add(1)
		return (*string)(nil), nil
	}
	for i := 0; i < 2; i++ {
		if _, err := coalescer.Do(context.Background(), "k", load); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if loads.Load() != 2 {
		t.Fatalf("loads = %d, want 2", loads.Load())
	}
}

func TestSingleflightCoalescerLoaderErrorDoesNotBlockRetry(t *testing.T) {
	t.Parallel()

	var loads atomic.Int32
	coalescer := loadguard.NewCoalescer(true)
	want := errors.New("load failed")
	load := func() (any, error) {
		loads.Add(1)
		return nil, want
	}
	if _, err := coalescer.Do(context.Background(), "k", load); !errors.Is(err, want) {
		t.Fatalf("first Do err = %v, want %v", err, want)
	}
	if _, err := coalescer.Do(context.Background(), "k", load); !errors.Is(err, want) {
		t.Fatalf("second Do err = %v, want %v", err, want)
	}
	if loads.Load() != 2 {
		t.Fatalf("loads = %d, want 2", loads.Load())
	}
}

func TestNoopCoalescerDoesNotCoalesce(t *testing.T) {
	t.Parallel()

	var loads atomic.Int32
	coalescer := loadguard.NoopCoalescer{}
	for i := 0; i < 3; i++ {
		if _, err := coalescer.Do(context.Background(), "k", func() (any, error) {
			loads.Add(1)
			return i, nil
		}); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if loads.Load() != 3 {
		t.Fatalf("loads = %d, want 3", loads.Load())
	}
}

// TestSingleflightCoalescerIgnoresContextCancel 合同：ctx 取消不传播到合并中的 fn（与 singleflight 语义一致）。
func TestSingleflightCoalescerIgnoresContextCancel(t *testing.T) {
	t.Parallel()

	coalescer := loadguard.NewCoalescer(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	value, err := coalescer.Do(ctx, "k", func() (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if value != "ok" {
		t.Fatalf("value = %v, want ok", value)
	}
}

func TestSingleflightCoalescerWaitsForInFlightOnCancel(t *testing.T) {
	t.Parallel()

	coalescer := loadguard.NewCoalescer(true)
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = coalescer.Do(context.Background(), "k", func() (any, error) {
			close(start)
			time.Sleep(20 * time.Millisecond)
			return "slow", nil
		})
		close(done)
	}()
	<-start
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	value, err := coalescer.Do(ctx, "k", func() (any, error) {
		return "other", nil
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if value != "slow" {
		t.Fatalf("value = %v, want slow (joined in-flight)", value)
	}
	<-done
}

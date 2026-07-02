package loadguard

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestGuardSingleflightCoalescesConcurrentLoads(t *testing.T) {
	guard := New[string, int](Policy{
		Singleflight: true,
		StaleOnError: false,
	}, func(v int) int { return v }, nil)

	var calls int32
	loader := func(context.Context) (int, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		return 42, nil
	}

	done := make(chan int, 8)
	for i := 0; i < 8; i++ {
		go func() {
			got, err := guard.Load(t.Context(), "k1", loader)
			if err != nil {
				t.Errorf("Load() error = %v", err)
				done <- -1
				return
			}
			done <- got
		}()
	}
	for i := 0; i < 8; i++ {
		if got := <-done; got != 42 {
			t.Fatalf("Load() = %d, want 42", got)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestGuardReturnsStaleOnLoadTimeout(t *testing.T) {
	guard := New[string, int](Policy{
		Singleflight: false,
		StaleOnError: true,
		LoadTimeout:  20 * time.Millisecond,
	}, func(v int) int { return v }, nil)

	first, err := guard.Load(t.Context(), "k1", func(context.Context) (int, error) {
		return 7, nil
	})
	if err != nil || first != 7 {
		t.Fatalf("first Load() = (%d,%v), want (7,nil)", first, err)
	}

	got, err := guard.Load(t.Context(), "k1", func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	})
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if got != 7 {
		t.Fatalf("second Load() = %d, want stale 7", got)
	}
}

func TestNoopCoalescerDoesNotCoalesce(t *testing.T) {
	coalescer := NoopCoalescer{}
	var calls int32
	loader := func() (any, error) {
		atomic.AddInt32(&calls, 1)
		return 1, nil
	}
	for i := 0; i < 3; i++ {
		if _, err := coalescer.Do(t.Context(), "k", loader); err != nil {
			t.Fatalf("Do() error = %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("loader calls = %d, want 3", got)
	}
}

func TestMemoryStaleStoreClone(t *testing.T) {
	store := NewMemoryStaleStore[string, []int](func(v []int) []int {
		out := make([]int, len(v))
		copy(out, v)
		return out
	})
	original := []int{1, 2}
	store.Remember("k", original)
	original[0] = 99
	got, ok := store.Load("k")
	if !ok || got[0] != 1 {
		t.Fatalf("Load() = %v, want isolated clone", got)
	}
}

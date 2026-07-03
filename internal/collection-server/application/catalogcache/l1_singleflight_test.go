package catalogcache

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/catalogreadthrough"
	"golang.org/x/sync/singleflight"
)

func TestL1SingleflightCoalescesConcurrentMiss(t *testing.T) {
	t.Parallel()

	var loads atomic.Int32
	var sf singleflight.Group
	cache := make(map[string]string)

	load := func() (string, error) {
		loads.Add(1)
		return "v", nil
	}
	const workers = 12
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := catalogreadthrough.ReadThrough(
				"k",
				func() (string, bool) { v, ok := cache["k"]; return v, ok },
				func(v string) { cache["k"] = v },
				load,
				func(v string) string { return v },
				&sf,
				true,
			)
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("readThrough: %v", err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
	_ = context.Background()
}

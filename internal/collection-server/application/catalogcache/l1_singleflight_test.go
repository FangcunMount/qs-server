package catalogcache

import (
	"context"
	"testing"
)

func TestL1SingleflightCoalescesConcurrentMiss(t *testing.T) {
	t.Parallel()

	fix := &catalogFixture{}
	const workers = 12
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := fix.readThrough(context.Background(), "k", true)
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("readThrough: %v", err)
		}
	}
	if fix.loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", fix.loads.Load())
	}
}

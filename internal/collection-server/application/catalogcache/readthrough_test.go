package catalogcache

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/catalogreadthrough"
	"golang.org/x/sync/singleflight"
)

type catalogFixture struct {
	cache map[string]string
	sf    singleflight.Group
	loads atomic.Int32
}

func (f *catalogFixture) get(key string) (string, bool) {
	v, ok := f.cache[key]
	return v, ok
}

func (f *catalogFixture) set(key, value string) {
	if f.cache == nil {
		f.cache = make(map[string]string)
	}
	f.cache[key] = value
}

func (f *catalogFixture) load(_ context.Context, key string) (string, error) {
	f.loads.Add(1)
	return "loaded:" + key, nil
}

func (f *catalogFixture) readThrough(ctx context.Context, key string, useSF bool) (string, error) {
	return catalogreadthrough.ReadThrough(
		key,
		func() (string, bool) { return f.get(key) },
		func(v string) { f.set(key, v) },
		func() (string, error) { return f.load(ctx, key) },
		func(v string) string { return v },
		&f.sf,
		useSF,
	)
}

func TestCatalogReadThroughPatternCacheHitSkipsLoad(t *testing.T) {
	fix := &catalogFixture{cache: map[string]string{"k": "cached"}}
	got, err := fix.readThrough(context.Background(), "k", true)
	if err != nil {
		t.Fatalf("readThrough: %v", err)
	}
	if got != "cached" {
		t.Fatalf("got %q, want cached", got)
	}
	if fix.loads.Load() != 0 {
		t.Fatalf("loads = %d, want 0", fix.loads.Load())
	}
}

func TestCatalogReadThroughPatternSingleflightCoalescesMiss(t *testing.T) {
	fix := &catalogFixture{}
	const workers = 8
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
	if got, ok := fix.get("k"); !ok || got != "loaded:k" {
		t.Fatalf("cache = %q ok=%v", got, ok)
	}
}

func TestCatalogReadThroughPatternWithoutSingleflightLoadsEachMiss(t *testing.T) {
	fix := &catalogFixture{}
	keys := []string{"a", "b", "c"}
	for _, key := range keys {
		if _, err := fix.readThrough(context.Background(), key, false); err != nil {
			t.Fatalf("readThrough(%s): %v", key, err)
		}
	}
	if fix.loads.Load() != 3 {
		t.Fatalf("loads = %d, want 3", fix.loads.Load())
	}
}

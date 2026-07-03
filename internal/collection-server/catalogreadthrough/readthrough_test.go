package catalogreadthrough_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/catalogreadthrough"
	"golang.org/x/sync/singleflight"
)

type fixture struct {
	mu    sync.Mutex
	cache map[string]string
	sf    singleflight.Group
	loads atomic.Int32
}

func (f *fixture) get(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.cache[key]
	return v, ok
}

func (f *fixture) set(key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cache == nil {
		f.cache = make(map[string]string)
	}
	f.cache[key] = value
}

func (f *fixture) readThrough(key string, useSF bool, load func() (string, error)) (string, error) {
	return catalogreadthrough.ReadThrough(
		key,
		func() (string, bool) { return f.get(key) },
		func(v string) { f.set(key, v) },
		load,
		func(v string) string { return v },
		&f.sf,
		useSF,
	)
}

func TestReadThroughCacheHitSkipsLoad(t *testing.T) {
	t.Parallel()

	fix := &fixture{cache: map[string]string{"k": "cached"}}
	got, err := fix.readThrough("k", true, func() (string, error) {
		fix.loads.Add(1)
		return "", nil
	})
	if err != nil {
		t.Fatalf("ReadThrough: %v", err)
	}
	if got != "cached" {
		t.Fatalf("got %q, want cached", got)
	}
	if fix.loads.Load() != 0 {
		t.Fatalf("loads = %d, want 0", fix.loads.Load())
	}
}

func TestReadThroughSingleflightCoalescesMiss(t *testing.T) {
	t.Parallel()

	fix := &fixture{}
	const workers = 8
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := fix.readThrough("k", true, func() (string, error) {
				fix.loads.Add(1)
				return "loaded", nil
			})
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("ReadThrough: %v", err)
		}
	}
	if fix.loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", fix.loads.Load())
	}
	if got, ok := fix.get("k"); !ok || got != "loaded" {
		t.Fatalf("cache = %q ok=%v", got, ok)
	}
}

func TestReadThroughWithoutSingleflightLoadsEachMiss(t *testing.T) {
	t.Parallel()

	fix := &fixture{}
	for _, key := range []string{"a", "b", "c"} {
		if _, err := fix.readThrough(key, false, func() (string, error) {
			fix.loads.Add(1)
			return "v:" + key, nil
		}); err != nil {
			t.Fatalf("ReadThrough(%s): %v", key, err)
		}
	}
	if fix.loads.Load() != 3 {
		t.Fatalf("loads = %d, want 3", fix.loads.Load())
	}
}

func TestReadThroughNilMissNotCached(t *testing.T) {
	t.Parallel()

	fix := &fixture{}
	load := func() (*string, error) {
		fix.loads.Add(1)
		return nil, nil
	}
	for i := 0; i < 2; i++ {
		if _, err := catalogreadthrough.ReadThrough(
			"k",
			func() (*string, bool) { return fix.getPtr("k") },
			func(v *string) { fix.setPtr("k", v) },
			load,
			func(v *string) *string { return v },
			&fix.sf,
			true,
		); err != nil {
			t.Fatalf("ReadThrough: %v", err)
		}
	}
	if fix.loads.Load() != 2 {
		t.Fatalf("loads = %d, want 2", fix.loads.Load())
	}
}

func TestReadThroughLoaderErrorDoesNotPolluteCache(t *testing.T) {
	t.Parallel()

	fix := &fixture{}
	want := errors.New("load failed")
	load := func() (string, error) {
		fix.loads.Add(1)
		return "", want
	}
	if _, err := fix.readThrough("k", true, load); !errors.Is(err, want) {
		t.Fatalf("first err = %v, want %v", err, want)
	}
	if _, err := fix.readThrough("k", true, load); !errors.Is(err, want) {
		t.Fatalf("second err = %v, want %v", err, want)
	}
	if fix.loads.Load() != 2 {
		t.Fatalf("loads = %d, want 2", fix.loads.Load())
	}
	if _, ok := fix.get("k"); ok {
		t.Fatal("error result must not be cached")
	}
}

func (f *fixture) getPtr(key string) (*string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.cache[key]
	if !ok {
		return nil, false
	}
	cp := v
	return &cp, true
}

func (f *fixture) setPtr(key string, v *string) {
	if v == nil {
		return
	}
	f.set(key, *v)
}

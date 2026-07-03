package catalogl1_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

func TestReadThroughCacheHitSkipsLoad(t *testing.T) {
	t.Parallel()

	var loads atomic.Int32
	got, err := catalogl1.ReadThrough("k", func() (string, bool) {
		return "cached", true
	}, func(string) {
		t.Fatal("set must not be called on cache hit")
	}, func() (string, error) {
		loads.Add(1)
		return "loaded", nil
	}, func(v string) string { return v }, loadguard.NewCoalescer(true), true)
	if err != nil {
		t.Fatalf("ReadThrough: %v", err)
	}
	if got != "cached" {
		t.Fatalf("got %q, want cached", got)
	}
	if loads.Load() != 0 {
		t.Fatalf("loads = %d, want 0", loads.Load())
	}
}

func TestReadThroughSingleflightCoalescesMiss(t *testing.T) {
	t.Parallel()

	var (
		mu    sync.Mutex
		cache = map[string]string{}
		loads atomic.Int32
	)
	coalescer := loadguard.NewCoalescer(true)
	get := func() (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		v, ok := cache["k"]
		return v, ok
	}
	set := func(v string) {
		mu.Lock()
		defer mu.Unlock()
		cache["k"] = v
	}
	load := func() (string, error) {
		loads.Add(1)
		return "loaded", nil
	}

	const workers = 8
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := catalogl1.ReadThrough("k", get, set, load, func(v string) string { return v }, coalescer, true)
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("ReadThrough: %v", err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
	if got, ok := get(); !ok || got != "loaded" {
		t.Fatalf("cache = %q ok=%v", got, ok)
	}
}

func TestReadThroughForgetNilMiss(t *testing.T) {
	var loads atomic.Int32
	coalescer := loadguard.NewCoalescer(true)
	load := func() (*string, error) {
		loads.Add(1)
		return nil, nil
	}
	cache := make(map[string]*string)
	set := func(v *string) { cache["k"] = v }
	for i := 0; i < 2; i++ {
		if _, err := catalogl1.ReadThrough("k", func() (*string, bool) {
			v, ok := cache["k"]
			return v, ok
		}, set, load, func(v *string) *string { return v }, coalescer, true); err != nil {
			t.Fatalf("readThrough: %v", err)
		}
	}
	if loads.Load() != 2 {
		t.Fatalf("loads = %d, want 2", loads.Load())
	}
}

func TestReadThroughClonesLoadedValueBeforeReturn(t *testing.T) {
	t.Parallel()

	type response struct {
		values []string
	}
	cache := make(map[string]*response)
	clone := func(v *response) *response {
		if v == nil {
			return nil
		}
		return &response{values: append([]string(nil), v.values...)}
	}

	got, err := catalogl1.ReadThrough("k", func() (*response, bool) {
		v, ok := cache["k"]
		return v, ok
	}, func(v *response) {
		cache["k"] = v
	}, func() (*response, error) {
		return &response{values: []string{"loaded"}}, nil
	}, clone, nil, false)
	if err != nil {
		t.Fatalf("ReadThrough: %v", err)
	}
	if got == nil || cache["k"] == nil {
		t.Fatalf("got=%+v cache=%+v, want both set", got, cache["k"])
	}
	if got == cache["k"] {
		t.Fatal("loaded value should be cloned before returning")
	}
	got.values[0] = "mutated"
	if cache["k"].values[0] != "loaded" {
		t.Fatalf("cache value mutated through returned clone: %+v", cache["k"])
	}
}

func TestReadThroughLoaderErrorDoesNotPolluteCache(t *testing.T) {
	coalescer := loadguard.NewCoalescer(true)
	want := errors.New("load failed")
	var loads atomic.Int32
	cache := make(map[string]string)
	load := func() (string, error) {
		loads.Add(1)
		return "", want
	}
	for i := 0; i < 2; i++ {
		if _, err := catalogl1.ReadThrough("k", func() (string, bool) {
			v, ok := cache["k"]
			return v, ok
		}, func(v string) { cache["k"] = v }, load, func(v string) string { return v }, coalescer, true); !errors.Is(err, want) {
			t.Fatalf("readThrough err = %v, want %v", err, want)
		}
	}
	if loads.Load() != 2 {
		t.Fatalf("loads = %d, want 2", loads.Load())
	}
	if _, ok := cache["k"]; ok {
		t.Fatal("error must not be cached")
	}
}

func TestReadThroughWithoutSingleflightSkipsCoalescer(t *testing.T) {
	var loads atomic.Int32
	load := func() (string, error) {
		loads.Add(1)
		return "v", nil
	}
	for i := 0; i < 2; i++ {
		if _, err := catalogl1.ReadThrough("k", func() (string, bool) {
			return "", false
		}, nil, load, func(v string) string { return v }, nil, false); err != nil {
			t.Fatalf("readThrough: %v", err)
		}
	}
	if loads.Load() != 2 {
		t.Fatalf("loads = %d, want 2", loads.Load())
	}
}

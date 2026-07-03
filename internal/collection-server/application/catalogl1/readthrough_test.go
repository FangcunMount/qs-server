package catalogl1_test

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

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

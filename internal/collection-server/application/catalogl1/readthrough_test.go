package catalogl1_test

import (
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

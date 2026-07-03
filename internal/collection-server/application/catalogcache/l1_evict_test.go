package catalogcache

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
)

func TestScaleL1EvictOnSignalClearsDetail(t *testing.T) {
	t.Parallel()

	cache := scale.NewLocalCatalogCache(scale.LocalCatalogCacheOptions{
		TTL:        time.Minute,
		MaxEntries: 16,
	})
	cache.SetDetail("demo", &scale.ScaleResponse{Code: "demo"})
	cache.EvictOnSignal("demo")
	if _, ok := cache.GetDetail("demo"); ok {
		t.Fatal("expected detail evicted after signal")
	}
}

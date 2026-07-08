package catalogcache

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

// TestL1PeekEquivalenceScaleDetail 锁定 HasCachedDetail 与 L1 内容一致（catalogL1Peek 依赖此语义）。
func TestL1PeekEquivalenceScaleDetail(t *testing.T) {
	t.Parallel()

	cache := scale.NewLocalCatalogCache(scale.LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 8})
	svc := scale.NewQueryService(nil, cache, false)
	if svc.HasCachedDetail("x") {
		t.Fatal("expected miss")
	}
	cache.SetDetail("x", &scale.ScaleResponse{Code: "x"})
	if !svc.HasCachedDetail("x") {
		t.Fatal("expected peek/hasCached true after set")
	}
}

func TestL1PeekEquivalencePersonalityDetailSmoke(t *testing.T) {
	t.Parallel()

	cache := typologymodel.NewLocalCatalogCache(typologymodel.LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 8})
	svc := typologymodel.NewQueryService(nil, cache, false)
	cache.SetDetail("p", &typologymodel.TypologyModelResponse{Code: "p"})
	if !svc.HasCachedDetail("p") {
		t.Fatal("expected personality peek equivalence")
	}
}

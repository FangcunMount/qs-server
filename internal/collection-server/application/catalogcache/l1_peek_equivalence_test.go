package catalogcache

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

func TestL1PeekEquivalencePersonalityDetailSmoke(t *testing.T) {
	t.Parallel()

	cache := typologymodel.NewLocalCatalogCache(typologymodel.LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 8})
	svc := typologymodel.NewQueryService(nil, cache, false)
	cache.SetDetail("p", &typologymodel.TypologyModelResponse{Code: "p"})
	if !svc.HasCachedDetail("p") {
		t.Fatal("expected personality peek equivalence")
	}
}

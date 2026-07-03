package scale

import (
	"context"

	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
)

// StartCacheSignalWatcher 订阅 scale_cache_changed 并失效 collection L1。
func StartCacheSignalWatcher(
	ctx context.Context,
	signaler *signalredis.Signaler[cachesignal.ScaleCacheChangedSignal],
	cache CatalogCache,
) {
	catalogl1.StartSignalWatcher(ctx, signaler, func(s cachesignal.ScaleCacheChangedSignal) string {
		return s.Code
	}, func(code string) {
		if cache != nil && code != "" {
			cache.EvictOnSignal(code)
		}
	}, "scale cache signal evicted")
}

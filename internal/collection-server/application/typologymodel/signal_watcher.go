package typologymodel

import (
	"context"

	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
)

// StartCacheSignalWatcher 订阅 typology_model_cache_changed 并失效 collection L1。
func StartCacheSignalWatcher(
	ctx context.Context,
	signaler *signalredis.Signaler[cachesignal.TypologyModelCacheChangedSignal],
	cache CatalogCache,
) {
	catalogl1.StartSignalWatcher(ctx, signaler, func(s cachesignal.TypologyModelCacheChangedSignal) string {
		return s.Code
	}, func(code string) {
		if cache != nil && code != "" {
			cache.EvictOnSignal(code)
		}
	}, "typology model cache signal evicted")
}

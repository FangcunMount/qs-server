package catalogcache

import (
	"time"

	localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"
)

// LocalTTLCacheOptions 构造带指标与 TTL 抖动的 L1 缓存选项。
func LocalTTLCacheOptions(kind string, ttl time.Duration, maxEntries int, jitterRatio float64) localcache.Options {
	return localcache.Options{
		TTL:            ttl,
		MaxEntries:     maxEntries,
		TTLJitterRatio: jitterRatio,
		OnHit: func() {
			RecordHit(kind)
		},
		OnMiss: func() {
			RecordMiss(kind)
		},
	}
}

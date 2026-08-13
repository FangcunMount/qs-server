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

// LocalTTLCacheOptionsWithRuntime keeps the legacy low-cardinality hit/miss
// label while adding code-owned capability and bucket labels for runtime data.
func LocalTTLCacheOptionsWithRuntime(kind, capability, bucket string, ttl time.Duration, maxEntries int, jitterRatio float64) localcache.Options {
	opts := LocalTTLCacheOptions(kind, ttl, maxEntries, jitterRatio)
	legacyHit, legacyMiss := opts.OnHit, opts.OnMiss
	opts.OnHit = func() {
		legacyHit()
		RecordRequest(capability, bucket, "hit")
	}
	opts.OnMiss = func() {
		legacyMiss()
		RecordRequest(capability, bucket, "miss")
	}
	RecordEntries(capability, bucket, 0)
	RecordMaxEntries(capability, bucket, maxEntries)
	opts.OnEntries = func(entries int) { RecordEntries(capability, bucket, entries) }
	opts.OnEviction = func(reason localcache.EvictionReason) { RecordEviction(capability, bucket, string(reason)) }
	return opts
}

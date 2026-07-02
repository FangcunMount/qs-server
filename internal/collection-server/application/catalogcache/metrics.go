package catalogcache

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var l1CacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "collection_l1_cache_hits_total",
	Help: "Total collection-server in-process L1 cache hits.",
}, []string{"kind"})

var l1CacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "collection_l1_cache_misses_total",
	Help: "Total collection-server in-process L1 cache misses.",
}, []string{"kind"})

// RecordHit 记录 L1 缓存命中。
func RecordHit(kind string) {
	if kind == "" {
		return
	}
	l1CacheHits.WithLabelValues(kind).Inc()
}

// RecordMiss 记录 L1 缓存未命中。
func RecordMiss(kind string) {
	if kind == "" {
		return
	}
	l1CacheMisses.WithLabelValues(kind).Inc()
}

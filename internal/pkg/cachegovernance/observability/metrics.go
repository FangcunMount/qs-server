package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	cacheGetTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_get_total",
			Help: "Total number of cache get results grouped by family, policy and result.",
		},
		[]string{"family", "policy", "result"},
	)
	cacheWriteTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_write_total",
			Help: "Total number of cache write-side operations grouped by family, policy, operation and result.",
		},
		[]string{"family", "policy", "op", "result"},
	)
	cacheOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "qs_cache_operation_duration_seconds",
			Help:    "Latency distribution for cache governance operations grouped by family, policy and operation.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"family", "policy", "op"},
	)
	cachePayloadBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "qs_cache_payload_bytes",
			Help:    "Payload size distribution for cache objects grouped by family, policy and stage.",
			Buckets: []float64{64, 128, 256, 512, 1024, 2 * 1024, 4 * 1024, 8 * 1024, 16 * 1024, 32 * 1024, 64 * 1024, 128 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024},
		},
		[]string{"family", "policy", "stage"},
	)
	cacheFamilyAvailable = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "qs_cache_family_available",
			Help: "Current Redis family availability grouped by component, family and profile.",
		},
		[]string{"component", "family", "profile"},
	)
	cacheFamilyDegradedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_family_degraded_total",
			Help: "Total number of family degraded transitions grouped by component, family, profile and reason.",
		},
		[]string{"component", "family", "profile", "reason"},
	)
	runtimeComponentReady = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "qs_runtime_component_ready",
			Help: "Current Redis runtime readiness grouped by component.",
		},
		[]string{"component"},
	)
	cacheWarmupDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "qs_cache_warmup_duration_seconds",
			Help:    "Warmup run latency grouped by trigger and result.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"trigger", "result"},
	)
	cacheHotsetSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "qs_cache_hotset_size",
			Help: "Current number of hotset members grouped by family and kind.",
		},
		[]string{"family", "kind"},
	)
	queryCacheVersionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_query_cache_version_total",
			Help: "Total number of query cache version-token operations grouped by kind, operation and result.",
		},
		[]string{"kind", "op", "result"},
	)
	cacheLockAcquireTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_lock_acquire_total",
			Help: "Total number of cache-backed lock acquire attempts grouped by lock name and result.",
		},
		[]string{"name", "result"},
	)
	cacheLockReleaseTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_lock_release_total",
			Help: "Total number of cache-backed lock release attempts grouped by lock name and result.",
		},
		[]string{"name", "result"},
	)
	cacheLockDegradedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_lock_degraded_total",
			Help: "Total number of degraded lock operations grouped by lock name and reason.",
		},
		[]string{"name", "reason"},
	)
)

func ObserveCacheGet(family, policy, result string) {
	cacheGetTotal.WithLabelValues(family, policy, result).Inc()
}

func ObserveCacheWrite(family, policy, op, result string) {
	cacheWriteTotal.WithLabelValues(family, policy, op, result).Inc()
}

func ObserveCacheOperationDuration(family, policy, op string, d time.Duration) {
	cacheOperationDuration.WithLabelValues(family, policy, op).Observe(d.Seconds())
}

func ObserveCachePayloadBytes(family, policy, stage string, size int) {
	if size < 0 {
		return
	}
	cachePayloadBytes.WithLabelValues(family, policy, stage).Observe(float64(size))
}

func SetCacheFamilyAvailable(component, family, profile string, available bool) {
	value := 0.0
	if available {
		value = 1
	}
	cacheFamilyAvailable.WithLabelValues(component, family, profile).Set(value)
}

func IncCacheFamilyDegraded(component, family, profile, reason string) {
	cacheFamilyDegradedTotal.WithLabelValues(component, family, profile, reason).Inc()
}

func SetRuntimeComponentReady(component string, ready bool) {
	value := 0.0
	if ready {
		value = 1
	}
	runtimeComponentReady.WithLabelValues(component).Set(value)
}

func ObserveWarmupDuration(trigger, result string, d time.Duration) {
	cacheWarmupDuration.WithLabelValues(trigger, result).Observe(d.Seconds())
}

func SetHotsetSize(family, kind string, size int64) {
	cacheHotsetSize.WithLabelValues(family, kind).Set(float64(size))
}

func ObserveQueryCacheVersion(kind, op, result string, d time.Duration) {
	queryCacheVersionTotal.WithLabelValues(kind, op, result).Inc()
	cacheOperationDuration.WithLabelValues("meta_hotset", kind, "version_"+op).Observe(d.Seconds())
}

func ObserveLockAcquire(name, result string) {
	cacheLockAcquireTotal.WithLabelValues(name, result).Inc()
}

func ObserveLockRelease(name, result string) {
	cacheLockReleaseTotal.WithLabelValues(name, result).Inc()
}

func ObserveLockDegraded(name, reason string) {
	cacheLockDegradedTotal.WithLabelValues(name, reason).Inc()
}

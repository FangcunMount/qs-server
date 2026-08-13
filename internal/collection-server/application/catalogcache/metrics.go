package catalogcache

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	KindPublishedModelDetail  = "published_model_detail"
	KindPublishedModelList    = "published_model_list"
	KindPublishedModelOptions = "published_model_options"
	KindAssessmentDetail      = "assessment_detail"
	KindAssessmentAccess      = "assessment_access"
)

var l1CacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "collection_l1_cache_hits_total",
	Help: "Total collection-server in-process L1 cache hits.",
}, []string{"kind"})

var l1CacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "collection_l1_cache_misses_total",
	Help: "Total collection-server in-process L1 cache misses.",
}, []string{"kind"})

var l1CacheEntries = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "collection_l1_cache_entries",
	Help: "Current collection-server L1 entries by capability and bucket.",
}, []string{"capability", "bucket"})

var l1CacheMaxEntries = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "collection_l1_cache_max_entries",
	Help: "Configured collection-server L1 maximum entries by capability and bucket.",
}, []string{"capability", "bucket"})

var l1CacheEvictions = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "collection_l1_cache_evictions_total",
	Help: "Total collection-server L1 removals by capability, bucket and reason.",
}, []string{"capability", "bucket", "reason"})

var l1CacheRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "collection_l1_cache_requests_total",
	Help: "Total collection-server L1 lookups by capability, bucket and result.",
}, []string{"capability", "bucket", "result"})

var l1SignalWatcherUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "collection_l1_signal_watcher_up",
	Help: "Whether a collection-server L1 signal watcher is currently running.",
}, []string{"capability"})

var l1SignalEvictions = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "collection_l1_signal_evictions_total",
	Help: "Total collection-server L1 signal eviction callbacks.",
}, []string{"capability"})

var l1SignalWatcherErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "collection_l1_signal_watcher_errors_total",
	Help: "Total collection-server L1 signal watcher errors.",
}, []string{"capability"})

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

func RecordEntries(capability, bucket string, entries int) {
	if capability != "" && bucket != "" {
		l1CacheEntries.WithLabelValues(capability, bucket).Set(float64(entries))
	}
}

func RecordMaxEntries(capability, bucket string, maxEntries int) {
	if capability != "" && bucket != "" {
		l1CacheMaxEntries.WithLabelValues(capability, bucket).Set(float64(maxEntries))
	}
}

func RecordEviction(capability, bucket, reason string) {
	if capability != "" && bucket != "" && reason != "" {
		l1CacheEvictions.WithLabelValues(capability, bucket, reason).Inc()
	}
}

func RecordRequest(capability, bucket, result string) {
	if capability != "" && bucket != "" && result != "" {
		l1CacheRequests.WithLabelValues(capability, bucket, result).Inc()
	}
}

func SetSignalWatcherUp(capability string, up bool) {
	if capability == "" {
		return
	}
	value := 0.0
	if up {
		value = 1
	}
	l1SignalWatcherUp.WithLabelValues(capability).Set(value)
}

func RecordSignalEviction(capability string) {
	if capability != "" {
		l1SignalEvictions.WithLabelValues(capability).Inc()
	}
}

func RecordSignalWatcherError(capability string) {
	if capability != "" {
		l1SignalWatcherErrors.WithLabelValues(capability).Inc()
	}
}

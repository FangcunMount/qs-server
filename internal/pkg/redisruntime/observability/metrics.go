package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	cacheFamilyAvailable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qs_cache_family_available",
		Help: "Current Redis family availability grouped by component, family and profile.",
	}, []string{"component", "family", "profile"})
	cacheFamilyDegradedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_cache_family_degraded_total",
		Help: "Total number of family degraded transitions grouped by component, family, profile and reason.",
	}, []string{"component", "family", "profile", "reason"})
	runtimeComponentReady = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qs_runtime_component_ready",
		Help: "Current Redis runtime readiness grouped by component.",
	}, []string{"component"})
	lockAcquireTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_cache_lock_acquire_total",
		Help: "Total number of cache-backed lock acquire attempts grouped by lock name and result.",
	}, []string{"name", "result"})
	lockReleaseTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_cache_lock_release_total",
		Help: "Total number of cache-backed lock release attempts grouped by lock name and result.",
	}, []string{"name", "result"})
	lockDegradedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_cache_lock_degraded_total",
		Help: "Total number of degraded lock operations grouped by lock name and reason.",
	}, []string{"name", "reason"})
)

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

func ObserveLockAcquire(name, result string)  { lockAcquireTotal.WithLabelValues(name, result).Inc() }
func ObserveLockRelease(name, result string)  { lockReleaseTotal.WithLabelValues(name, result).Inc() }
func ObserveLockDegraded(name, reason string) { lockDegradedTotal.WithLabelValues(name, reason).Inc() }

package leasemetrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// IR-R011: keep expired-lease pressure and reclaim latency observable so lease
// duration and reconcile cadence can be tuned without guessing.
var (
	ExpiredLeaseObservedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "qs",
		Subsystem: "interpretation",
		Name:      "expired_lease_observed_total",
		Help:      "Interpretation runs whose execution lease had expired when scanned (IR-R011).",
	})

	LeaseRecoveryDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "qs",
		Subsystem: "interpretation",
		Name:      "lease_recovery_duration_seconds",
		Help:      "Wall time from Interpretation Run lease expiry to successful reclaim (IR-R011).",
		Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600},
	})

	LeaseRecoveryTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "qs",
		Subsystem: "interpretation",
		Name:      "lease_recovery_total",
		Help:      "Successful Interpretation Run lease reclaims on the same attempt (IR-R011).",
	})
)

func ObserveExpiredLeases(count int) {
	if count <= 0 {
		return
	}
	ExpiredLeaseObservedTotal.Add(float64(count))
}

func ObserveRecovery(leaseExpiredAt, reclaimedAt time.Time) {
	if leaseExpiredAt.IsZero() || reclaimedAt.IsZero() || !reclaimedAt.After(leaseExpiredAt) {
		return
	}
	LeaseRecoveryDurationSeconds.Observe(reclaimedAt.Sub(leaseExpiredAt).Seconds())
	LeaseRecoveryTotal.Inc()
}

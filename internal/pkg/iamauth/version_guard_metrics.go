package iamauth

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	committedVersionReadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_authz_committed_version_read_total",
		Help: "IAM committed policy-version reads by bounded outcome.",
	}, []string{"result"})
	committedVersionReadDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "qs_authz_committed_version_read_duration_seconds",
		Help:    "IAM committed policy-version read latency by bounded outcome.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	}, []string{"result"})
	committedVersionProofStart = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "qs_authz_committed_version_proof_start_timestamp_seconds",
		Help: "Start time of the last successful IAM committed-version read; proof age is bounded from this instant.",
	})
	committedVersionProofRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qs_authz_committed_version_proof_rejected_total",
		Help: "Authorization proof checks rejected by bounded reason; excludes IAM business permission denials.",
	}, []string{"reason"})
)

func observeCommittedVersionRead(result string, elapsed time.Duration) {
	committedVersionReadTotal.WithLabelValues(result).Inc()
	committedVersionReadDuration.WithLabelValues(result).Observe(elapsed.Seconds())
}

package transaction

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	mongoTransactionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "mongo_transaction", Name: "total",
		Help: "Mongo transaction executions by stable boundary and outcome.",
	}, []string{"boundary", "outcome"})
	mongoTransactionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "mongo_transaction", Name: "duration_seconds",
		Help:    "Mongo transaction wall time, including admission and driver retries.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
	}, []string{"boundary", "outcome"})
	mongoTransactionAdmissionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "mongo_transaction", Name: "admission_wait_seconds",
		Help:    "Mongo transaction dependency admission wait by stable boundary and outcome.",
		Buckets: prometheus.ExponentialBuckets(0.0005, 2, 14),
	}, []string{"boundary", "outcome"})
	mongoTransactionCallbackAttempts = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "mongo_transaction", Name: "callback_attempts",
		Help:    "Number of callback executions performed by the Mongo driver for one transaction.",
		Buckets: []float64{1, 2, 3, 4, 5, 8, 13},
	}, []string{"boundary"})
	mongoTransactionCallbackRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "mongo_transaction", Name: "callback_retries_total",
		Help: "Mongo driver transaction callback retries beyond the first attempt.",
	}, []string{"boundary"})
)

func observeMongoAdmission(boundary, outcome string, duration time.Duration) {
	mongoTransactionAdmissionDuration.WithLabelValues(boundary, outcome).Observe(duration.Seconds())
}

func observeMongoTransaction(boundary, outcome string, duration time.Duration) {
	mongoTransactionTotal.WithLabelValues(boundary, outcome).Inc()
	mongoTransactionDuration.WithLabelValues(boundary, outcome).Observe(duration.Seconds())
}

func observeMongoCallbackAttempts(boundary string, attempts int) {
	if attempts > 0 {
		mongoTransactionCallbackAttempts.WithLabelValues(boundary).Observe(float64(attempts))
	}
	if attempts > 1 {
		mongoTransactionCallbackRetries.WithLabelValues(boundary).Add(float64(attempts - 1))
	}
}

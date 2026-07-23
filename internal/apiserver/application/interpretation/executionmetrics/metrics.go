package executionmetrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	ResultSuccess = "success"
	ResultError   = "error"
)

var (
	BuildDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs",
		Subsystem: "interpretation",
		Name:      "build_duration_seconds",
		Help:      "Interpretation report Builder wall time by builder identity and result (IR-R011).",
		Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
	}, []string{"builder_identity", "result"})

	RunDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs",
		Subsystem: "interpretation",
		Name:      "run_duration_seconds",
		Help:      "Interpretation started Run wall time through build and commit by builder identity and result (IR-R011).",
		Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"builder_identity", "result"})
)

func ObserveBuild(builderIdentity, result string, duration time.Duration) {
	BuildDurationSeconds.WithLabelValues(normalizeBuilderIdentity(builderIdentity), normalizeResult(result)).Observe(nonNegativeSeconds(duration))
}

func ObserveRun(builderIdentity, result string, duration time.Duration) {
	RunDurationSeconds.WithLabelValues(normalizeBuilderIdentity(builderIdentity), normalizeResult(result)).Observe(nonNegativeSeconds(duration))
}

func normalizeBuilderIdentity(value string) string {
	if value == "" {
		return "unresolved"
	}
	return value
}

func normalizeResult(value string) string {
	if value == ResultSuccess {
		return ResultSuccess
	}
	return ResultError
}

func nonNegativeSeconds(duration time.Duration) float64 {
	if duration < 0 {
		return 0
	}
	return duration.Seconds()
}

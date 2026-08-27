package execution

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	validationResultAccepted       = "accepted"
	validationResultOutputRejected = "output_rejected"
	validationResultSafetyRejected = "safety_rejected"
	validationResultSafetyError    = "safety_error"
	validationResultArtifactError  = "artifact_error"
	validationResultUnknown        = "unknown"
)

func observeOutputValidation(result string, duration time.Duration) {
	switch result {
	case validationResultAccepted, validationResultOutputRejected, validationResultSafetyRejected,
		validationResultSafetyError, validationResultArtifactError:
	default:
		result = validationResultUnknown
	}
	outputValidationTotal.WithLabelValues(result).Inc()
	outputValidationDurationSeconds.WithLabelValues(result).Observe(duration.Seconds())
}

var (
	outputValidationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation", Name: "validation_total",
		Help: "AI explanation post-Provider validation attempts by bounded result.",
	}, []string{"result"})
	outputValidationDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "ai_explanation", Name: "validation_duration_seconds",
		Help:    "AI explanation post-Provider schema, policy, safety and Artifact construction duration.",
		Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	}, []string{"result"})
)

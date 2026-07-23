package assessmentintake

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	intakeLookupFound           = "found"
	intakeLookupNotFound        = "not_found"
	intakeLookupDependencyError = "dependency_error"
	intakeLookupDuplicateHit    = "duplicate_hit"
)

var assessmentIntakeLookupTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_evaluation_assessment_intake_lookup_total",
	Help: "Assessment intake lookup classifications, including duplicate-then-refind recovery (EV-R006).",
}, []string{"result"})

func init() {
	for _, result := range []string{
		intakeLookupFound,
		intakeLookupNotFound,
		intakeLookupDependencyError,
		intakeLookupDuplicateHit,
	} {
		assessmentIntakeLookupTotal.WithLabelValues(result)
	}
}

func observeAssessmentIntakeLookup(result string) {
	assessmentIntakeLookupTotal.WithLabelValues(result).Inc()
}

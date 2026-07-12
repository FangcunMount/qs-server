package consistency

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	evaluationConsistencyMismatchTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "qs",
			Subsystem: "evaluation_consistency",
			Name:      "mismatch_total",
			Help:      "Total evaluation cross-store mismatches detected by the consistency reconciler.",
		},
		[]string{"kind"},
	)
	evaluationConsistencyDispositionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "qs",
			Subsystem: "evaluation_consistency",
			Name:      "disposition_total",
			Help:      "Total evaluation consistency mismatches by kind and audit disposition.",
		},
		[]string{"kind", "disposition"},
	)
)

func observeMismatch(kind MismatchKind) {
	evaluationConsistencyMismatchTotal.WithLabelValues(string(kind)).Inc()
}

func observeDisposition(kind MismatchKind, disposition string) {
	evaluationConsistencyDispositionTotal.WithLabelValues(string(kind), disposition).Inc()
}

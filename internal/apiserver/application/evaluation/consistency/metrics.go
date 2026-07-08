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
	evaluationConsistencyRepairTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "qs",
			Subsystem: "evaluation_consistency",
			Name:      "repair_total",
			Help:      "Total evaluation consistency repair attempts by kind and result.",
		},
		[]string{"kind", "result"},
	)
)

func observeMismatch(kind MismatchKind) {
	evaluationConsistencyMismatchTotal.WithLabelValues(string(kind)).Inc()
}

func observeRepair(kind MismatchKind, result string) {
	evaluationConsistencyRepairTotal.WithLabelValues(string(kind), result).Inc()
}

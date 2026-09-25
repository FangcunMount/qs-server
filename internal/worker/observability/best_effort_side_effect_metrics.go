package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type BestEffortSideEffectResult string

const (
	BestEffortCallSucceeded    BestEffortSideEffectResult = "call_succeeded"
	BestEffortCallFailed       BestEffortSideEffectResult = "call_failed"
	BestEffortResponseRejected BestEffortSideEffectResult = "response_rejected"
	BestEffortNotConfigured    BestEffortSideEffectResult = "not_configured"
)

var bestEffortSideEffectTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "worker", Name: "best_effort_side_effect_total",
	Help: "Best-effort Worker side-effect call outcomes; success is not final recipient delivery.",
}, []string{"event_type", "result"})

// ObserveBestEffortSideEffect accepts only the five approved event streams and
// bounded outcomes. Event IDs and recipient details belong in logs, never labels.
func ObserveBestEffortSideEffect(eventType string, result BestEffortSideEffectResult) {
	switch eventType {
	case "questionnaire.changed", "assessment_model.changed", "task.completed", "task.expired", "task.canceled":
	default:
		return
	}
	switch result {
	case BestEffortCallSucceeded, BestEffortCallFailed, BestEffortResponseRejected, BestEffortNotConfigured:
	default:
		return
	}
	bestEffortSideEffectTotal.WithLabelValues(eventType, string(result)).Inc()
}

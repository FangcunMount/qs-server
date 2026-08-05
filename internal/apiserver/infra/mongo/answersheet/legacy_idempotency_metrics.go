package answersheet

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var legacyIdempotencyFallbackLookupTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "answersheet",
	Name:      "legacy_idempotency_fallback_lookup_total",
	Help:      "Lookups against the legacy AnswerSheet idempotency collection after the embedded submit intent misses.",
})

var legacyIdempotencyFallbackHitTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "answersheet",
	Name:      "legacy_idempotency_fallback_hit_total",
	Help:      "Legacy AnswerSheet idempotency documents found by the compatibility fallback.",
})

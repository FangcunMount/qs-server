package resilienceplane

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var resilienceDecisionTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "qs_resilience_decision_total",
		Help: "Total resilience plane decisions.",
	},
	[]string{"component", "kind", "scope", "resource", "strategy", "outcome"},
)

type PrometheusObserver struct{}

func NewPrometheusObserver() *PrometheusObserver {
	return &PrometheusObserver{}
}

func (PrometheusObserver) ObserveDecision(_ context.Context, decision Decision) {
	subject := normalizeSubject(decision.Subject)
	resilienceDecisionTotal.WithLabelValues(
		subject.Component,
		decision.Kind.String(),
		subject.Scope,
		subject.Resource,
		subject.Strategy,
		decision.Outcome.String(),
	).Inc()
}

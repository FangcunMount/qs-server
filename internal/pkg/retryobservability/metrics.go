package retryobservability

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	LayerBusiness  = "business"
	LayerOutbox    = "outbox"
	LayerHold      = "hold"
	LayerTransport = "transport"

	AttemptInitial = "initial"
	AttemptRetry   = "retry"

	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)

var layerAttemptTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_retry_layer_attempt_total",
	Help: "Initial and retry attempts by bounded retry layer and outcome.",
}, []string{"layer", "component", "attempt_class", "origin", "outcome"})

func init() {
	for _, component := range []string{"evaluation", "interpretation"} {
		preinitialize(LayerBusiness, component, AttemptInitial, "initial")
		for _, origin := range []string{"automatic", "manual", "force", "lease_recovery"} {
			preinitialize(LayerBusiness, component, AttemptRetry, origin)
		}
	}
	for _, component := range []string{"mysql", "mongo", "outbox"} {
		preinitialize(LayerOutbox, component, AttemptInitial, "na")
		preinitialize(LayerOutbox, component, AttemptRetry, "na")
	}
	preinitialize(LayerHold, "retry_hold", AttemptInitial, "na")
	preinitialize(LayerHold, "retry_hold", AttemptRetry, "na")
	for _, component := range []string{"worker", "apiserver_consumer", "transport"} {
		preinitialize(LayerTransport, component, AttemptInitial, "na")
		preinitialize(LayerTransport, component, AttemptRetry, "na")
	}
}

func preinitialize(layer, component, attemptClass, origin string) {
	for _, outcome := range []string{OutcomeSuccess, OutcomeFailure} {
		layerAttemptTotal.WithLabelValues(layer, component, attemptClass, origin, outcome)
	}
}

func ObserveBusiness(component, origin, outcome string) {
	Observe(LayerBusiness, component, AttemptClassForOrigin(origin), origin, outcome)
}

func Observe(layer, component, attemptClass, origin, outcome string) {
	layer = normalizeLayer(layer)
	component = NormalizeComponent(layer, component)
	attemptClass = normalizeAttemptClass(attemptClass)
	origin = normalizeOrigin(layer, origin, attemptClass)
	outcome = normalizeOutcome(outcome)
	layerAttemptTotal.WithLabelValues(layer, component, attemptClass, origin, outcome).Inc()
}

func AttemptClassForOrigin(origin string) string {
	if strings.TrimSpace(origin) == "initial" {
		return AttemptInitial
	}
	return AttemptRetry
}

func AttemptClassForAttempt(attempt int) string {
	if attempt <= 1 {
		return AttemptInitial
	}
	return AttemptRetry
}

func NormalizeComponent(layer, value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch layer {
	case LayerBusiness:
		if normalized == "evaluation" || normalized == "interpretation" {
			return normalized
		}
		return "evaluation"
	case LayerOutbox:
		if strings.Contains(normalized, "mysql") {
			return "mysql"
		}
		if strings.Contains(normalized, "mongo") {
			return "mongo"
		}
		return "outbox"
	case LayerHold:
		return "retry_hold"
	case LayerTransport:
		if strings.Contains(normalized, "worker") {
			return "worker"
		}
		if normalized != "" && normalized != "transport" {
			return "apiserver_consumer"
		}
		return "transport"
	default:
		return "transport"
	}
}

func normalizeLayer(value string) string {
	switch value {
	case LayerBusiness, LayerOutbox, LayerHold, LayerTransport:
		return value
	default:
		return LayerTransport
	}
}

func normalizeAttemptClass(value string) string {
	if value == AttemptRetry {
		return AttemptRetry
	}
	return AttemptInitial
}

func normalizeOrigin(layer, origin, attemptClass string) string {
	if layer != LayerBusiness {
		return "na"
	}
	switch origin {
	case "initial", "automatic", "manual", "force", "lease_recovery":
		return origin
	default:
		if attemptClass == AttemptInitial {
			return "initial"
		}
		return "automatic"
	}
}

func normalizeOutcome(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "success", "succeeded", "published", "acked", "unknown_acked", "held", "hold_replayed":
		return OutcomeSuccess
	default:
		return OutcomeFailure
	}
}

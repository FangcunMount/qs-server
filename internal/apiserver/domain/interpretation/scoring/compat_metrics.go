package scoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Offline condition (IR-R005): remove each compatibility path after its series
// remains flat for a sustained window across all environments.
var factorInterpretationCompatTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "interpretation",
	Name:      "factor_interpretation_compat_total",
	Help:      "Factor interpretation compatibility use by legacy path.",
}, []string{"path"})

func observeFactorInterpretationCompatibility(path string) {
	factorInterpretationCompatTotal.WithLabelValues(path).Inc()
}

package evaluation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Offline condition (ops): when qs_evaluation_runtime_compat_total{source!="frozen"}
// stays at zero for a sustained window (e.g. 14d) across environments, remove the
// corresponding CompatibilityResolver branch (identity / legacy_typology /
// family_default_decision / draft_payload_format / assessment_model_ref).
var runtimeCompatTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs",
	Subsystem: "evaluation",
	Name:      "runtime_compat_total",
	Help:      "Evaluation RuntimeIdentity resolution by compatibility source and field.",
}, []string{"source", "field"})

func observeRuntimeCompat(hit CompatibilityHit, field string) {
	if hit.Source == "" || hit.Source == CompatibilitySourceNone {
		return
	}
	if field == "" {
		field = "unknown"
	}
	runtimeCompatTotal.WithLabelValues(string(hit.Source), field).Inc()
}

// ObserveRuntimeCompat records a CompatibilityResolver / migration fallback hit (EV-R008).
func ObserveRuntimeCompat(hit CompatibilityHit, field string) {
	observeRuntimeCompat(hit, field)
}

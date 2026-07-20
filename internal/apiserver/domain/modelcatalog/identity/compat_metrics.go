package identity

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Offline condition (ops / MC-R018):
// 1) inventory: audit_legacy_identities retained_read == 0
// 2) runtime: qs_modelcatalog_identity_write_policy_total{policy="retained_read"}
//    and qs_modelcatalog_identity_algorithm_fallback_total stay flat (rate≈0) for a
//    sustained window (e.g. 14d) across environments
// → then remove the corresponding retained-read / empty-algorithm fallback branch.
var (
	identityWritePolicyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs",
		Subsystem: "modelcatalog",
		Name:      "identity_write_policy_total",
		Help:      "Evaluation/model identity resolutions classified by write policy (MC-R018).",
	}, []string{"kind", "algorithm", "policy"})

	identityAlgorithmFallbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs",
		Subsystem: "modelcatalog",
		Name:      "identity_algorithm_fallback_total",
		Help:      "Empty or missing Algorithm filled by retained-read runtime fallbacks (MC-R018).",
	}, []string{"kind", "from", "to", "site"})
)

// ObserveWritePolicy records Kind/Algorithm write-policy classification at runtime.
func ObserveWritePolicy(kind Kind, algorithm Algorithm) {
	policy := ClassifyAlgorithmWritePolicy(kind, algorithm)
	identityWritePolicyTotal.WithLabelValues(string(kind), metricAlgorithm(algorithm), string(policy)).Inc()
}

// ObserveAlgorithmFallback records an empty/missing Algorithm filled at a named site.
func ObserveAlgorithmFallback(kind Kind, from, to Algorithm, site string) {
	if site == "" {
		site = "unknown"
	}
	identityAlgorithmFallbackTotal.WithLabelValues(
		string(kind), metricAlgorithm(from), metricAlgorithm(to), site,
	).Inc()
	ObserveWritePolicy(kind, from)
}

func metricAlgorithm(algorithm Algorithm) string {
	if algorithm == "" {
		return "_empty_"
	}
	return string(algorithm)
}

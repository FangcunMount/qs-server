package authobservability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// These counters describe verified REST authentication, not downstream authorization.
var authTokenSourceTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_auth_token_source_total",
	Help: "REST token carrier and authentication verification result; no token values.",
}, []string{"source", "result"})

var authClaimSourceTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "qs_auth_claim_source_total",
	Help: "Canonical identity selections from verified SDK claims at the REST boundary.",
}, []string{"field", "source"})

func init() {
	for _, source := range []string{"bearer", "bare", "query", "cookie", "missing", "unavailable"} {
		for _, result := range []string{"accepted", "rejected"} {
			authTokenSourceTotal.WithLabelValues(source, result)
		}
	}
	for field, sources := range map[string][]string{
		"user_id":    {"canonical", "missing"},
		"account_id": {"canonical", "missing"},
		"org_id":     {"canonical", "missing"},
	} {
		for _, source := range sources {
			authClaimSourceTotal.WithLabelValues(field, source)
		}
	}
}

// RecordTokenVerification is used only by the REST authentication boundary.
func RecordTokenVerification(source, result string) {
	switch source {
	case "bearer", "bare", "query", "cookie", "missing", "unavailable":
	default:
		source = "unavailable"
	}
	if result != "accepted" {
		result = "rejected"
	}
	authTokenSourceTotal.WithLabelValues(source, result).Inc()
}

// RecordVerifiedClaimSources receives selections from the verified REST mapper.
// Only canonical or missing selections are admitted.
func RecordVerifiedClaimSources(sources [3]string) {
	for index, field := range []string{"user_id", "account_id", "org_id"} {
		source := sources[index]
		valid := source == "canonical" || source == "missing"
		if !valid {
			source = "missing"
		}
		authClaimSourceTotal.WithLabelValues(field, source).Inc()
	}
}

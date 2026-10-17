package authobservability

import (
	"github.com/FangcunMount/qs-server/pkg/version"
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
	Help: "Actually selected identity claim sources after successful REST verification.",
}, []string{"field", "source"})

func init() {
	for _, source := range []string{"bearer", "bare", "query", "cookie", "missing", "unavailable"} {
		for _, result := range []string{"accepted", "rejected"} {
			authTokenSourceTotal.WithLabelValues(source, result)
		}
	}
	for field, sources := range map[string][]string{
		"user_id":    {"canonical", "missing", "extra_user_id", "extra_uid", "extra_sub"},
		"account_id": {"canonical", "missing", "extra_account_id", "extra_accountId", "extra_aid"},
		"org_id":     {"canonical", "missing", "extra_org_id", "extra_organization_id"},
	} {
		for _, source := range sources {
			authClaimSourceTotal.WithLabelValues(field, source)
		}
	}
	promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "qs_auth_compatibility_observation_info",
		Help:        "REST auth observation capability version 1 and compiled source SHA.",
		ConstLabels: prometheus.Labels{"source_sha": version.Get().GitCommit, "capability": "1"},
	}).Set(1)
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
// Field names and accepted aliases are fixed; arbitrary claims never become labels.
func RecordVerifiedClaimSources(sources [3]string) {
	for index, field := range []string{"user_id", "account_id", "org_id"} {
		source := sources[index]
		valid := source == "canonical" || source == "missing"
		switch field {
		case "user_id":
			valid = valid || source == "extra_user_id" || source == "extra_uid" || source == "extra_sub"
		case "account_id":
			valid = valid || source == "extra_account_id" || source == "extra_accountId" || source == "extra_aid"
		case "org_id":
			valid = valid || source == "extra_org_id" || source == "extra_organization_id"
		}
		if !valid {
			source = "missing"
		}
		authClaimSourceTotal.WithLabelValues(field, source).Inc()
	}
}

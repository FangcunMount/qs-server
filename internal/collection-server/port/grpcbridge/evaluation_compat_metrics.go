package grpcbridge

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var assessmentAuthorizationFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "collection",
	Subsystem: "evaluation_grpc",
	Name:      "assessment_authorization_fallback_total",
	Help:      "Total assessment authorization calls that fell back to the legacy detail RPC because the apiserver did not implement the authorization RPC.",
})

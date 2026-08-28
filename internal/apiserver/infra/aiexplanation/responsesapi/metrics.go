package responsesapi

import (
	"errors"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	providerPurposeGeneration        = "generation"
	providerPurposeSemanticEvaluator = "semantic_evaluator"
	providerPurposeUnknown           = "unknown"

	providerResultSuccess         = "success"
	providerResultRequestRejected = "request_rejected"
	providerResultCanceled        = "canceled"
	providerResultTimeout         = "timeout"
	providerResultRateLimited     = "rate_limited"
	providerResultRefusal         = "refusal"
	providerResultTransportError  = "transport_error"
	providerResultInvalidResponse = "invalid_response"
	providerResultUnknown         = "result_unknown"
	providerResultError           = "error"
)

// observeProviderInvocation exposes only bounded purpose/result labels. Model,
// route, organization, Assessment, Testee, request and invocation identities
// are deliberately excluded from Prometheus labels.
func observeProviderInvocation(schemaVersion string, duration time.Duration, response *appport.ProviderResponse, err error) {
	purpose := providerMetricPurpose(schemaVersion)
	result := providerMetricResult(err)
	providerRequestsTotal.WithLabelValues(purpose, result).Inc()
	providerDurationSeconds.WithLabelValues(purpose, result).Observe(duration.Seconds())
	if err == nil && response != nil {
		providerTokensTotal.WithLabelValues(purpose, "input").Add(float64(response.Receipt.InputTokens))
		providerTokensTotal.WithLabelValues(purpose, "output").Add(float64(response.Receipt.OutputTokens))
	}
}

func providerMetricPurpose(schemaVersion string) string {
	switch schemaVersion {
	case aiexplanation.OutputSchemaVersionV1:
		return providerPurposeGeneration
	case aiexplanation.SemanticEvaluationOutputSchemaVersionV1:
		return providerPurposeSemanticEvaluator
	default:
		return providerPurposeUnknown
	}
}

func providerMetricResult(err error) string {
	if err == nil {
		return providerResultSuccess
	}
	var providerErr *appport.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return providerResultError
	}
	if providerErr.ResultUnknown {
		return providerResultUnknown
	}
	switch providerErr.Kind {
	case domainrun.FailureKindProviderTimeout:
		return providerResultTimeout
	case domainrun.FailureKindProviderRateLimit:
		return providerResultRateLimited
	case domainrun.FailureKindProviderRefusal:
		return providerResultRefusal
	}
	switch providerErr.Code {
	case "provider_request_invalid", "provider_request_encode_failed", "provider_request_build_failed":
		return providerResultRequestRejected
	case "provider_request_cancelled", "provider_response_cancelled":
		return providerResultCanceled
	case "provider_transport_error", "provider_response_read_failed", "provider_server_error":
		return providerResultTransportError
	case "provider_response_invalid", "provider_response_too_large", "provider_response_id_missing",
		"provider_model_mismatch", "provider_response_failed", "provider_response_incomplete",
		"provider_response_not_terminal", "provider_response_status_invalid", "provider_output_cardinality_invalid",
		"provider_output_token_limit", "provider_usage_invalid":
		return providerResultInvalidResponse
	default:
		return providerResultError
	}
}

var (
	providerRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "requests_total",
		Help: "AI explanation Provider requests by bounded purpose and result.",
	}, []string{"purpose", "result"})
	providerDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "duration_seconds",
		Help:    "AI explanation Provider request duration by bounded purpose and result.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 40, 80},
	}, []string{"purpose", "result"})
	providerTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "tokens_total",
		Help: "Provider-reported AI explanation token usage by bounded purpose and direction.",
	}, []string{"purpose", "direction"})
)

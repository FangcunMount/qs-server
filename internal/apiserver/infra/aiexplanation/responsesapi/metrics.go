package responsesapi

import (
	"errors"
	"strings"
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
	providerFailureCodeOther      = "other"

	providerResponseStatusCompleted            = "completed"
	providerResponseStatusIncompleteTokenLimit = "incomplete_token_limit"
	providerResponseStatusIncompleteContent    = "incomplete_content_filter"
	providerResponseStatusIncompleteOther      = "incomplete_other"
	providerResponseStatusFailed               = "failed"
	providerResponseStatusCanceled             = "canceled"
	providerResponseStatusNotTerminal          = "not_terminal"
	providerResponseStatusInvalid              = "invalid"

	providerResponseShapeSingleMessageOutputText = "single_message_output_text"
	providerResponseShapeSingleMessageEmptyText  = "single_message_empty_output_text"
	providerResponseShapeSingleMessageNoText     = "single_message_no_output_text"
	providerResponseShapeMultipleMessages        = "multiple_messages"
	providerResponseShapeNoMessage               = "no_message"
	providerResponseShapeRefusal                 = "refusal"
)

// observeProviderInvocation exposes only bounded purpose/result labels. Model,
// route, organization, Assessment, Testee, request and invocation identities
// are deliberately excluded from Prometheus labels.
func observeProviderInvocation(schemaVersion string, duration time.Duration, response *appport.ProviderResponse, err error) {
	purpose := providerMetricPurpose(schemaVersion)
	result := providerMetricResult(err)
	providerRequestsTotal.WithLabelValues(purpose, result).Inc()
	providerDurationSeconds.WithLabelValues(purpose, result).Observe(duration.Seconds())
	if err != nil {
		providerFailuresTotal.WithLabelValues(purpose, result, providerMetricFailureCode(err)).Inc()
	}
	if err == nil && response != nil {
		providerTokensTotal.WithLabelValues(purpose, "input").Add(float64(response.Receipt.InputTokens))
		providerTokensTotal.WithLabelValues(purpose, "output").Add(float64(response.Receipt.OutputTokens))
	}
}

// observeDecodedProviderResponse records only reviewed finite response-shape
// labels and numeric usage. Provider-generated text, remote error messages and
// every internal or external identity are deliberately excluded. Unlike the
// success-only tokens_total metric, response_tokens_total also makes token
// usage from an incomplete response visible for capacity and truncation
// diagnosis.
func observeDecodedProviderResponse(schemaVersion string, response responsesAPIResponse) {
	purpose := providerMetricPurpose(schemaVersion)
	status := providerMetricResponseStatus(response)
	shape := providerMetricResponseShape(response.Output)
	providerResponseShapesTotal.WithLabelValues(purpose, status, shape).Inc()
	if response.Usage == nil || response.Usage.InputTokens < 0 || response.Usage.OutputTokens < 0 {
		return
	}
	providerResponseTokensTotal.WithLabelValues(purpose, status, "input").Add(float64(response.Usage.InputTokens))
	providerResponseTokensTotal.WithLabelValues(purpose, status, "output").Add(float64(response.Usage.OutputTokens))
}

func providerMetricResponseStatus(response responsesAPIResponse) string {
	switch response.Status {
	case "completed":
		return providerResponseStatusCompleted
	case "incomplete":
		if response.IncompleteDetails == nil {
			return providerResponseStatusIncompleteOther
		}
		switch strings.ToLower(strings.TrimSpace(response.IncompleteDetails.Reason)) {
		case "max_output_tokens":
			return providerResponseStatusIncompleteTokenLimit
		case "content_filter":
			return providerResponseStatusIncompleteContent
		default:
			return providerResponseStatusIncompleteOther
		}
	case "failed":
		return providerResponseStatusFailed
	case "cancelled":
		return providerResponseStatusCanceled
	case "queued", "in_progress":
		return providerResponseStatusNotTerminal
	default:
		return providerResponseStatusInvalid
	}
}

func providerMetricResponseShape(items []outputItem) string {
	messageCount := 0
	outputTextCount := 0
	nonEmptyOutputTextCount := 0
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		messageCount++
		for _, content := range item.Content {
			switch content.Type {
			case "refusal":
				return providerResponseShapeRefusal
			case "output_text":
				outputTextCount++
				if strings.TrimSpace(content.Text) != "" {
					nonEmptyOutputTextCount++
				}
			}
		}
	}
	switch {
	case messageCount == 0:
		return providerResponseShapeNoMessage
	case messageCount > 1:
		return providerResponseShapeMultipleMessages
	case outputTextCount == 0:
		return providerResponseShapeSingleMessageNoText
	case nonEmptyOutputTextCount == 0:
		return providerResponseShapeSingleMessageEmptyText
	default:
		return providerResponseShapeSingleMessageOutputText
	}
}

// providerMetricFailureCode keeps the error-code label finite. New Provider
// error codes must be reviewed and added explicitly; arbitrary remote values
// and wrapped error messages collapse to "other".
func providerMetricFailureCode(err error) string {
	var providerErr *appport.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return providerFailureCodeOther
	}
	switch providerErr.Code {
	case "provider_request_invalid", "provider_request_encode_failed", "provider_request_build_failed",
		"provider_request_cancelled", "provider_request_rejected", "provider_authentication_failed",
		"provider_transport_error", "provider_response_read_failed", "provider_server_error",
		"provider_response_invalid", "provider_response_too_large", "provider_response_id_missing",
		"provider_model_mismatch", "provider_response_failed", "provider_response_incomplete",
		"provider_response_cancelled", "provider_response_not_terminal", "provider_response_status_invalid",
		"provider_output_cardinality_invalid", "provider_output_token_limit", "provider_usage_invalid",
		"provider_rate_limited", "provider_timeout", "provider_refusal":
		return providerErr.Code
	default:
		return providerFailureCodeOther
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
	providerFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "failures_total",
		Help: "AI explanation Provider failures by bounded purpose, result, and reviewed code.",
	}, []string{"purpose", "result", "code"})
	providerTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "tokens_total",
		Help: "Provider-reported AI explanation token usage by bounded purpose and direction.",
	}, []string{"purpose", "direction"})
	providerResponseShapesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "response_shapes_total",
		Help: "Decoded Provider responses by bounded purpose, terminal status, and safe structural shape.",
	}, []string{"purpose", "status", "shape"})
	providerResponseTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "response_tokens_total",
		Help: "Provider-reported token usage for decoded responses by bounded purpose, terminal status, and direction.",
	}, []string{"purpose", "status", "direction"})
)

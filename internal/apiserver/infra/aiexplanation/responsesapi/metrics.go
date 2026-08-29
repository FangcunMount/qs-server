package responsesapi

import (
	"encoding/json"
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
	providerResponseShapeSingleStrictToolCall    = "single_strict_tool_call"
	providerResponseShapeMultipleMessages        = "multiple_messages"
	providerResponseShapeNoMessage               = "no_message"
	providerResponseShapeRefusal                 = "refusal"

	providerOutputEnvelopeJSONObject    = "json_object"
	providerOutputEnvelopeMarkdownFence = "markdown_fence"
	providerOutputEnvelopeNonObjectJSON = "non_object_json"
	providerOutputEnvelopeInvalidJSON   = "invalid_json"

	providerOutputNormalizationUnchanged         = "unchanged"
	providerOutputNormalizationMarkdownUnwrapped = "markdown_json_fence_unwrapped"
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

// observeProviderOutputEnvelope records the bounded outer shape of completed
// output before schema validation. It never records generated text. This makes
// Provider wire-contract drift such as Markdown fences directly observable.
func observeProviderOutputEnvelope(schemaVersion, output string) {
	providerOutputEnvelopesTotal.WithLabelValues(
		providerMetricPurpose(schemaVersion), providerMetricOutputEnvelope(output),
	).Inc()
}

// observeProviderOutputNormalization records only whether a reviewed,
// deterministic wire-envelope compatibility rule was applied. It never
// records generated text or Provider identities.
func observeProviderOutputNormalization(schemaVersion string, normalized bool) {
	result := providerOutputNormalizationUnchanged
	if normalized {
		result = providerOutputNormalizationMarkdownUnwrapped
	}
	providerOutputNormalizationsTotal.WithLabelValues(providerMetricPurpose(schemaVersion), result).Inc()
}

func providerMetricOutputEnvelope(output string) string {
	trimmed := strings.TrimSpace(output)
	if strings.HasPrefix(trimmed, "```") {
		return providerOutputEnvelopeMarkdownFence
	}
	if !json.Valid([]byte(trimmed)) {
		return providerOutputEnvelopeInvalidJSON
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &object); err == nil && object != nil {
		return providerOutputEnvelopeJSONObject
	}
	return providerOutputEnvelopeNonObjectJSON
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
	if details := response.Usage.OutputTokensDetails; details != nil && details.ReasoningTokens >= 0 && details.ReasoningTokens <= response.Usage.OutputTokens {
		providerResponseTokensTotal.WithLabelValues(purpose, status, "reasoning").Add(float64(details.ReasoningTokens))
	}
}

// observeDeepSeekStrictToolResponse records only the finite terminal status,
// strict-tool structural shape and numeric usage. Function arguments and every
// Provider-generated string remain excluded from metrics.
func observeDeepSeekStrictToolResponse(schemaVersion string, response deepSeekChatResponse) {
	purpose := providerMetricPurpose(schemaVersion)
	status := providerResponseStatusInvalid
	shape := providerResponseShapeNoMessage
	if len(response.Choices) == 1 {
		choice := response.Choices[0]
		switch choice.FinishReason {
		case "tool_calls":
			status = providerResponseStatusCompleted
		case "length":
			status = providerResponseStatusIncompleteTokenLimit
		case "content_filter":
			status = providerResponseStatusIncompleteContent
		case "insufficient_system_resource":
			status = providerResponseStatusFailed
		default:
			status = providerResponseStatusInvalid
		}
		switch {
		case strings.TrimSpace(choice.Message.Refusal) != "":
			shape = providerResponseShapeRefusal
		case len(choice.Message.ToolCalls) == 1:
			shape = providerResponseShapeSingleStrictToolCall
		case len(choice.Message.ToolCalls) > 1:
			shape = providerResponseShapeMultipleMessages
		default:
			shape = providerResponseShapeSingleMessageNoText
		}
	}
	providerResponseShapesTotal.WithLabelValues(purpose, status, shape).Inc()
	if response.Usage == nil || response.Usage.PromptTokens < 0 || response.Usage.CompletionTokens < 0 {
		return
	}
	providerResponseTokensTotal.WithLabelValues(purpose, status, "input").Add(float64(response.Usage.PromptTokens))
	providerResponseTokensTotal.WithLabelValues(purpose, status, "output").Add(float64(response.Usage.CompletionTokens))
	if details := response.Usage.CompletionTokensDetails; details != nil &&
		details.ReasoningTokens >= 0 && details.ReasoningTokens <= response.Usage.CompletionTokens {
		providerResponseTokensTotal.WithLabelValues(purpose, status, "reasoning").Add(float64(details.ReasoningTokens))
	}
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
	providerOutputEnvelopesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "output_envelopes_total",
		Help: "Completed Provider output text by bounded purpose and safe outer JSON envelope classification.",
	}, []string{"purpose", "envelope"})
	providerOutputNormalizationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "ai_explanation_provider", Name: "output_normalizations_total",
		Help: "Completed Provider outputs by bounded deterministic envelope normalization result.",
	}, []string{"purpose", "result"})
)

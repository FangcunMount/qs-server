package evaluation

import (
	"strings"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const promptEvaluationFailureLabelOther = "other"

func observePromptEvaluationAttemptFailure(failure *domainevaluation.AttemptFailure) {
	if failure == nil {
		return
	}
	promptEvaluationAttemptFailuresTotal.WithLabelValues(
		promptEvaluationFailureStageLabel(failure.Stage),
		promptEvaluationFailureCodeLabel(failure.Code),
	).Inc()
}

func promptEvaluationFailureStageLabel(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "input_assembly", "prompt_render", "provider_execution", "evidence_capture",
		"output_normalization", "deterministic_evaluation", "output_validation", "semantic_evaluation":
		return value
	default:
		return promptEvaluationFailureLabelOther
	}
}

// promptEvaluationFailureCodeLabel keeps the Prometheus label finite. Local
// failure codes and reviewed Provider adapter codes must be added explicitly;
// arbitrary remote values collapse to "other".
func promptEvaluationFailureCodeLabel(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "synthetic_input_invalid", "prompt_render_failed", "provider_execution_failed",
		"provider_response_missing", "provider_output_too_large", "provider_receipt_invalid",
		"output_normalization_failed", "candidate_evaluation_failed", "candidate_receipts_invalid",
		"provider_output_schema_invalid", "provider_output_object_required", "provider_output_json_syntax_invalid",
		"provider_output_unknown_field", "provider_output_field_type_invalid", "provider_output_json_decode_invalid",
		"provider_output_trailing_content", "provider_output_content_contract_invalid",
		"semantic_evaluation_failed", "semantic_receipt_invalid",
		"provider_result_unknown",
		"provider_request_invalid", "provider_request_encode_failed", "provider_request_build_failed",
		"provider_request_cancelled", "provider_request_rejected", "provider_authentication_failed",
		"provider_transport_error", "provider_response_read_failed", "provider_server_error",
		"provider_response_invalid", "provider_response_too_large", "provider_response_id_missing",
		"provider_model_mismatch", "provider_response_failed", "provider_response_incomplete",
		"provider_response_cancelled", "provider_response_not_terminal", "provider_response_status_invalid",
		"provider_output_cardinality_invalid", "provider_output_token_limit", "provider_usage_invalid",
		"provider_rate_limited", "provider_timeout", "provider_refusal":
		return value
	default:
		return promptEvaluationFailureLabelOther
	}
}

var promptEvaluationAttemptFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "qs", Subsystem: "ai_explanation_prompt_evaluation", Name: "attempt_failures_total",
	Help: "Durably recorded Prompt evaluation technical failures by bounded stage and code.",
}, []string{"stage", "code"})

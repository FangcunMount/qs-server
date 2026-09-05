package evaluation

import (
	ai "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSemanticProtocolRecoveryClassification(t *testing.T) {
	for _, tc := range []struct {
		name, providerCode, shape, status, code string
		unknown, retryable                      bool
	}{
		{"completed without message", "provider_output_cardinality_invalid", "no_message", "completed", domain.SemanticProviderNoMessage, false, true},
		{"multiple messages", "provider_output_cardinality_invalid", "multiple_messages", "completed", domain.SemanticProviderFailed, false, false},
		{"incomplete without message", "provider_output_cardinality_invalid", "no_message", "incomplete", domain.SemanticProviderFailed, false, false},
		{"unknown cannot auto retry", "provider_output_cardinality_invalid", "no_message", "completed", domain.SemanticResultUnknown, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			diagnostics := &ai.ProviderFailureDiagnostics{Code: tc.providerCode, RequestID: "resp_test", ResponseStatus: tc.status, ResponseShape: tc.shape}
			outcome := SemanticEvaluationOutcome{FinishedAt: now, ProviderCallCount: 1, ProviderDiagnostics: diagnostics,
				Failure: &domain.AttemptFailure{Code: domain.SemanticProviderFailed, SafeMessage: "Provider 未返回结构化消息", ResultUnknown: tc.unknown}}
			got := semanticExecutionV2(domain.EvidenceExecutionCheckpoint{ID: "execution:1", CandidateID: "candidate:1", InvocationID: "invocation:1", ExecutionOrdinal: 1}, now, outcome, nil, nil, domain.SemanticEvaluatorSpec{})
			require.Equal(t, tc.code, got.Failure.Code)
			require.Equal(t, tc.retryable, got.Failure.Retryable)
			require.Equal(t, diagnostics, got.Failure.ProviderDiagnostics)
			require.NoError(t, got.Failure.Validate())
			diagnostics.RequestID = "changed"
			require.Equal(t, "resp_test", got.Failure.ProviderDiagnostics.RequestID)
			require.Equal(t, tc.retryable, domain.CurrentEvaluationExecutionPolicy().Recovery.AllowsAutomaticRetry(*got.Failure))
		})
	}
}

func TestSemanticRateLimitMatchesRecoverySelector(t *testing.T) {
	now := time.Now().UTC()
	outcome := SemanticEvaluationOutcome{FinishedAt: now, ProviderCallCount: 1,
		ProviderDiagnostics: &ai.ProviderFailureDiagnostics{Code: "provider_rate_limited"},
		Failure:             &domain.AttemptFailure{Code: domain.SemanticProviderFailed, SafeMessage: "rate limited", Retryable: true}}
	got := semanticExecutionV2(domain.EvidenceExecutionCheckpoint{ID: "execution:1"}, now, outcome, nil, nil, domain.SemanticEvaluatorSpec{})
	require.Equal(t, domain.SemanticProviderRateLimited, got.Failure.Code)
	require.True(t, domain.CurrentEvaluationExecutionPolicy().Recovery.AllowsAutomaticRetry(*got.Failure))
}

package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReleaseGatePolicyVersionSelectsKnownSemantics(t *testing.T) {
	policy := CurrentReleaseGatePolicy()
	require.Equal(t, "v2", policy.Version)
	for _, version := range []string{"v1", "v2"} {
		policy.Version = version
		require.NoError(t, policy.Validate())
	}
	policy.Version = "v3"
	require.Error(t, policy.Validate(), "unknown gate semantics must not be applied to evidence")
}

func TestGatePolicyV2DoesNotCountRecoveredProviderFailuresAsSuccess(t *testing.T) {
	evidence := completeEvidenceV2ForReview(t)
	freezeGateVersion(t, &evidence, "v2")
	generation := &evidence.GenerationExecutions[0]
	generation.RawOutput = nil
	generation.Failure.Stage = FailureStageGenerationExecution
	generation.Failure.Kind = FailureKindInfrastructureExecution
	generation.Failure.Code = "provider_rate_limited"
	generation.Failure.Retryable = true
	generation.Failure.Disposition = FailureDispositionRetryGeneration
	semantic := &evidence.SemanticExecutions[0]
	semantic.ProviderReceipt = nil
	semantic.RawOutput = nil
	semantic.Failure.Code = SemanticProviderFailed

	gate, err := evidence.EvaluateGate(evidence.Audit.CreatedAt.Add(3 * time.Hour))
	require.NoError(t, err)
	require.Equal(t, 70, gate.Metrics[0].Numerator)
	require.Equal(t, 72, gate.Metrics[0].Denominator)
	require.False(t, gate.GatePasses["G3"])
	require.Contains(t, gate.Reasons, EvidenceGateReason{
		Gate: "G3", Code: "infrastructure_success_rate_below_threshold", Detail: "Provider execution reliability is below the frozen threshold",
	})
	require.Equal(t, 35, gate.Metrics[1].Numerator)
	require.Equal(t, 35, gate.Metrics[1].Denominator)
}

func TestGatePolicyV2SeparatesProviderSuccessFromOutputConformance(t *testing.T) {
	evidence := completeEvidenceV2ForReview(t)
	freezeGateVersion(t, &evidence, "v2")
	generation := &evidence.GenerationExecutions[0]
	receipt := providerReceipt(generation.InvocationID, "generation-failed-output")
	generation.ProviderReceipt = &receipt

	gate, err := evidence.EvaluateGate(evidence.Audit.CreatedAt.Add(3 * time.Hour))
	require.NoError(t, err)
	require.Equal(t, 72, gate.Metrics[0].Numerator)
	require.Equal(t, 72, gate.Metrics[0].Denominator)
	require.Equal(t, 35, gate.Metrics[1].Numerator)
	require.Equal(t, 36, gate.Metrics[1].Denominator)
	require.Equal(t, 35, gate.Metrics[2].Numerator)
	require.Equal(t, 36, gate.Metrics[2].Denominator)

	// A receipt alone is not proof when the execution rejected its identity.
	evidence.SemanticExecutions[0].Failure.Code = SemanticReceiptInvalid
	gate, err = evidence.EvaluateGate(evidence.Audit.CreatedAt.Add(3 * time.Hour))
	require.NoError(t, err)
	require.Equal(t, 71, gate.Metrics[0].Numerator)
	generation.ProviderReceipt.InvocationID = "different-invocation"
	gate, err = evidence.EvaluateGate(evidence.Audit.CreatedAt.Add(3 * time.Hour))
	require.NoError(t, err)
	require.Equal(t, 70, gate.Metrics[0].Numerator)
}

func TestGatePolicyV2AppliesCaseThresholdsWithoutWeakeningHardAssertions(t *testing.T) {
	for _, test := range []struct {
		name       string
		failed     []int
		hard       bool
		missing    bool
		wantPassed bool
		wantCode   string
	}{
		{name: "one ordinary failure", failed: []int{0}, wantPassed: true},
		{name: "32 of 35 across cases", failed: []int{0, 5, 10}, wantPassed: true},
		{name: "case below 4 of 5", failed: []int{0, 1}, wantCode: "case_assertion_stability_failed"},
		{name: "all case candidates failed", failed: []int{0, 1, 2, 3, 4}, wantCode: "case_assertion_stability_failed"},
		{name: "overall below 32 of 35", failed: []int{0, 5, 10, 15}, wantCode: "case_assertion_overall_failed"},
		{name: "one hard failure", failed: []int{0}, hard: true, wantCode: "candidate_hard_assertion_failed"},
		{name: "missing case evidence", failed: []int{0}, missing: true, wantCode: "candidate_case_assertion_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			evidence := completeEvidenceV2ForReview(t)
			freezeGateVersion(t, &evidence, "v2")
			for _, index := range test.failed {
				if test.missing {
					evidence.Slots[index].Candidate.Assertions = evidence.Slots[index].Candidate.Assertions[:1]
					continue
				}
				assertion := &evidence.Slots[index].Candidate.Assertions[1]
				assertion.Status = AssertionFailed
				assertion.Hard = test.hard
			}
			gate, err := evidence.EvaluateGate(evidence.Audit.CreatedAt.Add(3 * time.Hour))
			require.NoError(t, err)
			require.Equal(t, test.wantPassed, gate.GatePasses["G4"], "%+v", gate.Reasons)
			if test.wantCode != "" {
				var codes []string
				for _, reason := range gate.Reasons {
					codes = append(codes, reason.Code)
				}
				require.Contains(t, codes, test.wantCode)
			}
		})
	}
}

func TestGatePolicyV1RestoresHistoricalFinalResult(t *testing.T) {
	evidence := completeEvidenceV2ForReview(t)
	evidence.Slots[0].Candidate.Assertions[1].Status = AssertionFailed
	at := evidence.Audit.CreatedAt.Add(3 * time.Hour)
	require.NoError(t, evidence.Finalize("user:approver", "release_gate_evaluated", at))
	// These values and reasons are the historical v1 calculation, independent of
	// CurrentReleaseGatePolicy. Changing them would invalidate stored terminal Runs.
	evidence.GateResult = &EvidenceGateResult{
		EvaluatedAt: at, Passed: false,
		GatePasses: map[string]bool{"G1": true, "G2": true, "G3": false, "G4": false, "G5": true},
		Metrics: []EvidenceGateMetric{
			{Name: "infrastructure_success_rate", Numerator: 72, Denominator: 72, Value: 1, Threshold: 0.98},
			{Name: "generation_contract_conformance_rate", Numerator: 35, Denominator: 36, Value: 35.0 / 36, Threshold: 0.95},
			{Name: "semantic_execution_success_rate", Numerator: 35, Denominator: 36, Value: 35.0 / 36, Threshold: 0.98},
		},
		Reasons: []EvidenceGateReason{
			{Gate: "G3", Code: "semantic_execution_success_rate_below_threshold", Detail: "Semantic execution reliability is below the frozen threshold"},
			{Gate: "G4", Code: "candidate_case_assertion_failed", Detail: "Candidate did not pass all case-scoped assertions", EvidenceRefs: []string{evidence.Slots[0].Candidate.ID}},
		},
	}
	restored, err := RestorePromptEvaluationEvidenceV2(evidence, evidence.Version(), nil)
	require.NoError(t, err)
	require.Equal(t, EvidenceStatusRejected, restored.Status)
	require.Equal(t, evidence.GateResult, restored.GateResult)
}

func freezeGateVersion(t *testing.T, evidence *PromptEvaluationEvidenceV2, version string) {
	t.Helper()
	evidence.GatePolicy.Version = version
	fingerprint, err := evidence.GatePolicy.Fingerprint()
	require.NoError(t, err)
	evidence.Release.GatePolicy.Version = version
	evidence.Release.GatePolicy.Fingerprint = fingerprint
	evidence.Release.Fingerprint, err = evidence.Release.ExpectedFingerprint()
	require.NoError(t, err)
}

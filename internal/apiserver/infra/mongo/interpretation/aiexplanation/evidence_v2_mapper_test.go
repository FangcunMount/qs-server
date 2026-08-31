package aiexplanation

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"

	domainai "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestPromptEvaluationEvidenceV2MapperRoundTripPreservesTerminalOutputAndCheckpoint(t *testing.T) {
	evidence := newMapperEvidenceV2(t)
	mapper := NewMapper()

	po, err := mapper.PromptEvaluationEvidenceV2ToPO(evidence)
	require.NoError(t, err)
	require.Equal(t, PromptEvaluationEvidenceVersionV2, po.EvidenceVersion)
	require.Equal(t, evidence.Release.Fingerprint.String(), po.ActiveReleaseKey)
	require.Equal(t, "7", po.ActiveExecutionOrgKey)
	require.Len(t, po.GenerationExecutions, 1)
	require.NotNil(t, po.Execution)

	raw, err := bson.Marshal(po)
	require.NoError(t, err)
	var decoded PromptEvaluationEvidenceV2PO
	require.NoError(t, bson.Unmarshal(raw, &decoded))
	require.Equal(t, po.GenerationExecutions[0].RawOutput, decoded.GenerationExecutions[0].RawOutput)

	restored, err := mapper.PromptEvaluationEvidenceV2ToDomain(&decoded)
	require.NoError(t, err)
	require.Equal(t, evidence.Clone(), restored.Clone())
	require.Equal(t, evidence.Version(), restored.Version())
	require.Equal(t, evidence.Execution(), restored.Execution())

	decoded.ActiveReleaseKey = ""
	_, err = mapper.PromptEvaluationEvidenceV2ToDomain(&decoded)
	require.ErrorContains(t, err, "indexed projection is inconsistent")
}

func newMapperEvidenceV2(t *testing.T) *domainevaluation.PromptEvaluationEvidenceV2 {
	t.Helper()
	createdAt := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	executionPolicy := mapperExecutionPolicyV2()
	gatePolicy := mapperGatePolicyV2()
	executionFingerprint, err := executionPolicy.Fingerprint()
	require.NoError(t, err)
	gateFingerprint, err := gatePolicy.Fingerprint()
	require.NoError(t, err)
	ref := func(id string) domainevaluation.FrozenContractRef {
		return domainevaluation.FrozenContractRef{ID: id, Version: "v1", Fingerprint: domainai.NewFingerprint([]byte(id))}
	}
	release := domainevaluation.EvidenceReleaseIdentity{
		Suite: ref("suite"), Prompt: ref("prompt"), Profile: ref("profile"),
		InputSchema: ref("input-schema"), OutputSchema: ref("output-schema"), GenerationRoute: ref("generation-route"),
		SemanticPrompt: ref("semantic-prompt"), SemanticOutputSchema: ref("semantic-output-schema"), SemanticRoute: ref("semantic-route"),
		ExecutionPolicy: domainevaluation.FrozenContractRef{ID: executionPolicy.PolicyID, Version: executionPolicy.Version, Fingerprint: executionFingerprint},
		GatePolicy:      domainevaluation.FrozenContractRef{ID: gatePolicy.PolicyID, Version: gatePolicy.Version, Fingerprint: gateFingerprint},
	}
	release.Fingerprint, err = release.ExpectedFingerprint()
	require.NoError(t, err)
	caseIDs := make([]string, domainevaluation.RequiredGenerationCaseCount)
	for index := range caseIDs {
		caseIDs[index] = fmt.Sprintf("case-%d", index+1)
	}
	evidence, err := domainevaluation.NewPromptEvaluationEvidenceV2(
		meta.ID(9920), release, executionPolicy, gatePolicy, caseIDs, "preflight-ineligible",
		7, "user:42", "验证 v2 Mongo 映射", createdAt,
	)
	require.NoError(t, err)
	require.NoError(t, evidence.Transition(domainevaluation.EvidenceStatusCollecting, "capacity_reserved", "system:runner", nil, createdAt.Add(time.Minute)))
	preflightAt := createdAt.Add(2 * time.Minute)
	require.NoError(t, evidence.CompletePreflight(domainevaluation.PreflightCaseEvidence{
		CaseID: "preflight-ineligible", Status: domainevaluation.PreflightEvidencePassed, EvaluatedAt: &preflightAt,
		RejectionReason: "insufficient_eligible_dimensions",
		Assertions: []domainevaluation.AssertionReceipt{
			{Type: "provider_call_count", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed},
			{Type: "rejection_reason", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: domainevaluation.AssertionPassed},
		},
	}))
	claimedAt := createdAt.Add(3 * time.Minute)
	checkpoint := domainevaluation.EvidenceExecutionCheckpoint{
		ID: "generation:v2:1", Kind: domainevaluation.EvidenceExecutionGeneration,
		CaseID: "case-1", SlotOrdinal: 1, ExecutionOrdinal: 1,
		Owner: "worker:1", InvocationID: "generation-v2-invocation:1", Phase: domainevaluation.AttemptExecutionPrepared,
		ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(2 * time.Minute),
	}
	require.NoError(t, evidence.BeginNextExecution(checkpoint))
	dispatchedAt := claimedAt.Add(10 * time.Second)
	require.NoError(t, evidence.MarkExecutionDispatching(checkpoint.Owner, dispatchedAt))
	finishedAt := dispatchedAt.Add(10 * time.Second)
	rawOutput := []byte(`{"schema_version":"ai-explanation-output/v1","summary":"ok"}`)
	receipt := domainai.ProviderReceipt{
		InvocationID: checkpoint.InvocationID, RequestID: "generation-request:1", Provider: "deepseek", Model: "deepseek-v4-pro",
		InputTokens: 100, OutputTokens: 200, Latency: time.Second,
	}
	require.NoError(t, evidence.CompleteGenerationExecution(checkpoint.Owner, "candidate:v2:1", []domainevaluation.AssertionReceipt{{
		Type: "output_schema_valid", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true,
		Evaluator: "deterministic-v1", Status: domainevaluation.AssertionPassed,
	}}, domainevaluation.CandidateGenerationExecution{
		ID: checkpoint.ID, CaseID: checkpoint.CaseID, SlotOrdinal: checkpoint.SlotOrdinal, ExecutionOrdinal: checkpoint.ExecutionOrdinal,
		InvocationID: checkpoint.InvocationID, Status: domainevaluation.ExecutionStatusSucceeded,
		StartedAt: dispatchedAt, FinishedAt: &finishedAt, ProviderCallCount: 1, ProviderReceipt: &receipt,
		RawOutput: rawOutput, NormalizedOutput: rawOutput, NormalizedOutputFingerprint: domainai.NewFingerprint(rawOutput),
	}))
	semanticAt := createdAt.Add(4 * time.Minute)
	require.NoError(t, evidence.BeginNextExecution(domainevaluation.EvidenceExecutionCheckpoint{
		ID: "semantic:v2:1", Kind: domainevaluation.EvidenceExecutionSemantic,
		CaseID: "case-1", SlotOrdinal: 1, CandidateID: "candidate:v2:1", ExecutionOrdinal: 1,
		Owner: "worker:1", InvocationID: "semantic-v2-invocation:1", Phase: domainevaluation.AttemptExecutionPrepared,
		ClaimedAt: semanticAt, LeaseExpiresAt: semanticAt.Add(2 * time.Minute),
	}))
	return evidence
}

func mapperExecutionPolicyV2() domainevaluation.EvaluationExecutionPolicy {
	return domainevaluation.EvaluationExecutionPolicy{
		SchemaVersion: domainevaluation.EvaluationExecutionPolicySchemaVersionV1,
		PolicyID:      "release-evaluation-bounded-recovery", Version: "v1",
		SlotPolicy: domainevaluation.EvaluationSlotPolicy{
			RequiredGenerationCases:   domainevaluation.RequiredGenerationCaseCount,
			RequiredCandidatesPerCase: domainevaluation.RequiredRepetitionsPerCase,
			RequiredPreflightCases:    1, CandidateSelection: domainevaluation.CandidateSelectionFirstContractConformant,
		},
		Generation: domainevaluation.GenerationExecutionBudget{MaxExecutionsPerSlot: 2, MaxExecutionsPerRun: 70},
		Semantic:   domainevaluation.SemanticExecutionBudget{MaxExecutionsPerCandidate: 2, MaxExecutionsPerRun: 70},
		Recovery: domainevaluation.EvaluationRecoveryPolicy{
			AutoRetryableStageCodes: []domainevaluation.FailureSelector{
				{Stage: domainevaluation.FailureStageGenerationExecution, Code: "provider_rate_limited"},
				{Stage: domainevaluation.FailureStageSemanticEvaluation, Code: "semantic_provider_rate_limited"},
			},
			ManualRecoveryStageCodes:                   []domainevaluation.FailureSelector{{Stage: domainevaluation.FailureStageGenerationExecution, Code: "provider_result_unknown"}},
			ResultUnknownRequiresManualAcknowledgement: true,
		},
	}
}

func mapperGatePolicyV2() domainevaluation.ReleaseGatePolicy {
	return domainevaluation.ReleaseGatePolicy{
		SchemaVersion: domainevaluation.ReleaseGatePolicySchemaVersionV1, PolicyID: "release-gates", Version: "v1",
		ReleaseIdentity: domainevaluation.ReleaseIdentityGatePolicy{
			RequiredComponents: []domainevaluation.ReleaseIdentityComponent{
				domainevaluation.ReleaseComponentSuite, domainevaluation.ReleaseComponentPrompt, domainevaluation.ReleaseComponentProfile,
				domainevaluation.ReleaseComponentInputSchema, domainevaluation.ReleaseComponentOutputSchema,
				domainevaluation.ReleaseComponentGenerationRoute, domainevaluation.ReleaseComponentSemanticPrompt,
				domainevaluation.ReleaseComponentSemanticOutputSchema, domainevaluation.ReleaseComponentSemanticRoute,
				domainevaluation.ReleaseComponentExecutionPolicy,
			},
			RequireFingerprintMatch: true,
		},
		SampleCompleteness: domainevaluation.SampleCompletenessGatePolicy{
			RequiredGenerationCases:              domainevaluation.RequiredGenerationCaseCount,
			RequiredCandidatesPerCase:            domainevaluation.RequiredRepetitionsPerCase,
			RequiredCandidateCount:               domainevaluation.RequiredGenerationAttempts,
			RequiredSemanticReceiptsPerCandidate: 1, RejectUnresolvedResultUnknown: true, RejectBudgetOverrun: true,
		},
		ExecutionReliability: domainevaluation.ExecutionReliabilityGatePolicy{
			MinInfrastructureSuccessRate: 0.98, MinGenerationContractConformanceRate: 0.95, MinSemanticExecutionSuccessRate: 0.98,
			InfrastructureDenominator: "dispatched_provider_executions", GenerationContractDenominator: "definite_output_generation_executions",
			SemanticExecutionDenominator: "dispatched_semantic_executions", IncludeResultUnknownInInfrastructureDenominator: true,
		},
		CandidateQuality: domainevaluation.CandidateQualityGatePolicy{
			MinAssertionPassesPerCase: 4, MinAssertionPassesOverall: 32,
			MinimumSemanticScores:              domainevaluation.SemanticScoreThresholds{Faithfulness: 4, CrossDimensionQuality: 3, SuggestionActionability: 3, AudienceClarity: 3, Concision: 3},
			MinimumSemanticAverages:            domainevaluation.SemanticScoreThresholds{Faithfulness: 4.5, CrossDimensionQuality: 4, SuggestionActionability: 4, AudienceClarity: 4, Concision: 4},
			HardAssertionFailureRejectsRelease: true,
		},
		HumanAccountability: domainevaluation.HumanAccountabilityGatePolicy{
			RequiredRoles:               []domainevaluation.ReviewRole{domainevaluation.ReviewRoleAssessmentSemantics, domainevaluation.ReviewRoleSafetyProduct},
			RequiredReviewsPerCandidate: 2, RequiredReviewCount: 70,
			RequireDistinctReviewersPerCandidate: true, RequireReason: true, AnyRejectionRejectsRelease: true,
		},
		ApprovalRule: "all_gates_must_pass",
	}
}

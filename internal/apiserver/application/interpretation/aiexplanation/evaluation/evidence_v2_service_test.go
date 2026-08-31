package evaluation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainai "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestEvidenceV2ServiceClaimsDomainSelectedActionWithCAS(t *testing.T) {
	evidence := newServiceEvidenceV2(t)
	repository := &evidenceV2RepositoryStub{value: evidence}
	service, err := NewEvidenceV2Service(repository)
	require.NoError(t, err)
	claimedAt := evidence.Audit.CreatedAt.Add(3 * time.Minute)
	expectedVersion := evidence.Version()

	updated, err := service.ClaimNextExecution(context.Background(), evidence.RunID, ClaimEvidenceV2ExecutionCommand{
		ExecutionID: "generation:service:1", Owner: "worker:1", InvocationID: "generation-service-invocation:1",
		ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, expectedVersion, repository.expectedVersion)
	require.Equal(t, expectedVersion+1, updated.Version())
	require.Equal(t, "case-1", updated.Execution().CaseID)
	require.Equal(t, 1, updated.Execution().SlotOrdinal)
	require.Equal(t, domainevaluation.EvidenceExecutionGeneration, updated.Execution().Kind)

	saveCalls := repository.saveCalls
	_, err = service.ClaimNextExecution(context.Background(), evidence.RunID, ClaimEvidenceV2ExecutionCommand{
		ExecutionID: "generation:service:2", Owner: "worker:2", InvocationID: "generation-service-invocation:2",
		ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	})
	require.Error(t, err)
	require.Equal(t, saveCalls, repository.saveCalls)

	repository.saveErr = domainevaluation.ErrConflict
	_, err = service.MarkExecutionDispatching(context.Background(), evidence.RunID, "worker:1", claimedAt.Add(10*time.Second))
	require.ErrorIs(t, err, domainevaluation.ErrConflict)
}

type evidenceV2RepositoryStub struct {
	value           *domainevaluation.PromptEvaluationEvidenceV2
	expectedVersion int64
	saveCalls       int
	saveErr         error
}

func (r *evidenceV2RepositoryStub) CreateEvidenceV2(_ context.Context, value *domainevaluation.PromptEvaluationEvidenceV2) error {
	if r.value != nil {
		return domainevaluation.ErrAlreadyExists
	}
	r.value = cloneServiceEvidenceV2(value)
	return nil
}

func (r *evidenceV2RepositoryStub) SaveEvidenceV2(_ context.Context, value *domainevaluation.PromptEvaluationEvidenceV2, expectedVersion int64) error {
	r.expectedVersion = expectedVersion
	r.saveCalls++
	if r.saveErr != nil {
		return r.saveErr
	}
	if r.value == nil || r.value.Version() != expectedVersion {
		return domainevaluation.ErrConflict
	}
	r.value = cloneServiceEvidenceV2(value)
	return nil
}

func (r *evidenceV2RepositoryStub) FindEvidenceV2ByID(_ context.Context, id meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	if r.value == nil || r.value.RunID != id {
		return nil, domainevaluation.ErrNotFound
	}
	return cloneServiceEvidenceV2(r.value), nil
}

func cloneServiceEvidenceV2(value *domainevaluation.PromptEvaluationEvidenceV2) *domainevaluation.PromptEvaluationEvidenceV2 {
	if value == nil {
		return nil
	}
	restored, err := domainevaluation.RestorePromptEvaluationEvidenceV2(value.Clone(), value.Version(), value.Execution())
	if err != nil {
		panic(err)
	}
	return restored
}

func newServiceEvidenceV2(t *testing.T) *domainevaluation.PromptEvaluationEvidenceV2 {
	t.Helper()
	createdAt := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	executionPolicy := serviceExecutionPolicyV2()
	gatePolicy := serviceGatePolicyV2()
	executionFingerprint, err := executionPolicy.Fingerprint()
	require.NoError(t, err)
	gateFingerprint, err := gatePolicy.Fingerprint()
	require.NoError(t, err)
	ref := func(id string) domainevaluation.FrozenContractRef {
		return domainevaluation.FrozenContractRef{ID: id, Version: "v1", Fingerprint: domainai.NewFingerprint([]byte(id))}
	}
	release := domainevaluation.EvidenceReleaseIdentity{
		Suite: ref("suite"), Prompt: ref("prompt"), Profile: ref("profile"), InputSchema: ref("input-schema"),
		OutputSchema: ref("output-schema"), GenerationRoute: ref("generation-route"), SemanticPrompt: ref("semantic-prompt"),
		SemanticOutputSchema: ref("semantic-output-schema"), SemanticRoute: ref("semantic-route"),
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
		meta.ID(9930), release, executionPolicy, gatePolicy, caseIDs, "preflight-ineligible",
		7, "user:42", "验证 v2 Application CAS", createdAt,
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
	return evidence
}

func serviceExecutionPolicyV2() domainevaluation.EvaluationExecutionPolicy {
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

func serviceGatePolicyV2() domainevaluation.ReleaseGatePolicy {
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

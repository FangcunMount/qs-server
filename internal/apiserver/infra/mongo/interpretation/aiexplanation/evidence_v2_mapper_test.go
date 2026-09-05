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

func TestPromptEvaluationEvidenceV2MapperRoundTripsCompleteThirtyFiveSlotDocument(t *testing.T) {
	evidence := completeMapperEvidenceV2(t)
	mapper := NewMapper()

	po, err := mapper.PromptEvaluationEvidenceV2ToPO(evidence)
	require.NoError(t, err)
	raw, err := bson.Marshal(po)
	require.NoError(t, err)
	var decoded PromptEvaluationEvidenceV2PO
	require.NoError(t, bson.Unmarshal(raw, &decoded))
	restored, err := mapper.PromptEvaluationEvidenceV2ToDomain(&decoded)
	require.NoError(t, err)
	require.Equal(t, domainevaluation.EvidenceStatusAwaitingReview, restored.Status)
	require.Len(t, restored.Slots, domainevaluation.RequiredGenerationAttempts)
	require.Len(t, restored.GenerationExecutions, domainevaluation.RequiredGenerationAttempts)
	require.Len(t, restored.SemanticExecutions, domainevaluation.RequiredGenerationAttempts)
	require.Nil(t, restored.Execution())
	require.NoError(t, restored.Validate())
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

func completeMapperEvidenceV2(t *testing.T) *domainevaluation.PromptEvaluationEvidenceV2 {
	t.Helper()
	evidence := newMapperEvidenceV2(t)
	for evidence.Status == domainevaluation.EvidenceStatusCollecting {
		if checkpoint := evidence.Execution(); checkpoint != nil {
			completeMapperSemanticExecutionV2(t, evidence, *checkpoint)
			continue
		}
		action, err := evidence.NextAction()
		require.NoError(t, err)
		switch action.Kind {
		case domainevaluation.EvidenceNextActionGeneration:
			completeMapperGenerationExecutionV2(t, evidence, action)
		case domainevaluation.EvidenceNextActionSemantic:
			checkpoint := beginMapperSemanticExecutionV2(t, evidence, action)
			completeMapperSemanticExecutionV2(t, evidence, checkpoint)
		default:
			t.Fatalf("unexpected next action while completing v2 evidence: %#v", action)
		}
	}
	require.Equal(t, domainevaluation.EvidenceStatusAwaitingReview, evidence.Status)
	require.Len(t, evidence.Slots, domainevaluation.RequiredGenerationAttempts)
	require.Len(t, evidence.GenerationExecutions, domainevaluation.RequiredGenerationAttempts)
	require.Len(t, evidence.SemanticExecutions, domainevaluation.RequiredGenerationAttempts)
	require.NoError(t, evidence.Validate())
	return evidence
}

func completeMapperGenerationExecutionV2(t *testing.T, evidence *domainevaluation.PromptEvaluationEvidenceV2, action domainevaluation.EvidenceNextAction) {
	t.Helper()
	sequence := len(evidence.GenerationExecutions) + 1
	claimedAt := evidence.LastModifiedAt().Add(time.Minute)
	checkpoint := domainevaluation.EvidenceExecutionCheckpoint{
		ID: fmt.Sprintf("generation:full:%d", sequence), Kind: domainevaluation.EvidenceExecutionGeneration,
		CaseID: action.CaseID, SlotOrdinal: action.SlotOrdinal, ExecutionOrdinal: action.ExecutionOrdinal,
		Owner: "worker:full", InvocationID: fmt.Sprintf("generation:full:invocation:%d", sequence),
		Phase: domainevaluation.AttemptExecutionPrepared, ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	}
	require.NoError(t, evidence.BeginNextExecution(checkpoint))
	dispatchedAt := claimedAt.Add(time.Second)
	require.NoError(t, evidence.MarkExecutionDispatching(checkpoint.Owner, dispatchedAt))
	finishedAt := dispatchedAt.Add(time.Second)
	rawOutput := []byte(fmt.Sprintf(`{"schema_version":"ai-explanation-output/v1","summary":"candidate %d"}`, sequence))
	receipt := mapperProviderReceiptV2(checkpoint.InvocationID, fmt.Sprintf("generation:full:request:%d", sequence))
	require.NoError(t, evidence.CompleteGenerationExecution(checkpoint.Owner, fmt.Sprintf("candidate:full:%d", sequence), []domainevaluation.AssertionReceipt{
		{Type: "output_schema_valid", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "deterministic-v1", Status: domainevaluation.AssertionPassed},
		{Type: "case_goal", Scope: domainevaluation.AssertionScopeCase, Ordinal: 1, Evaluator: "semantic-v1", Status: domainevaluation.AssertionPendingSemantic},
	}, domainevaluation.CandidateGenerationExecution{
		ID: checkpoint.ID, CaseID: checkpoint.CaseID, SlotOrdinal: checkpoint.SlotOrdinal, ExecutionOrdinal: checkpoint.ExecutionOrdinal,
		InvocationID: checkpoint.InvocationID, Status: domainevaluation.ExecutionStatusSucceeded,
		StartedAt: dispatchedAt, FinishedAt: &finishedAt, ProviderCallCount: 1, ProviderReceipt: &receipt,
		RawOutput: rawOutput, NormalizedOutput: rawOutput, NormalizedOutputFingerprint: domainai.NewFingerprint(rawOutput),
	}))
}

func beginMapperSemanticExecutionV2(t *testing.T, evidence *domainevaluation.PromptEvaluationEvidenceV2, action domainevaluation.EvidenceNextAction) domainevaluation.EvidenceExecutionCheckpoint {
	t.Helper()
	sequence := len(evidence.SemanticExecutions) + 1
	claimedAt := evidence.LastModifiedAt().Add(time.Minute)
	checkpoint := domainevaluation.EvidenceExecutionCheckpoint{
		ID: fmt.Sprintf("semantic:full:%d", sequence), Kind: domainevaluation.EvidenceExecutionSemantic,
		CaseID: action.CaseID, SlotOrdinal: action.SlotOrdinal, CandidateID: action.CandidateID, ExecutionOrdinal: action.ExecutionOrdinal,
		Owner: "worker:full", InvocationID: fmt.Sprintf("semantic:full:invocation:%d", sequence),
		Phase: domainevaluation.AttemptExecutionPrepared, ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	}
	require.NoError(t, evidence.BeginNextExecution(checkpoint))
	return checkpoint
}

func completeMapperSemanticExecutionV2(t *testing.T, evidence *domainevaluation.PromptEvaluationEvidenceV2, checkpoint domainevaluation.EvidenceExecutionCheckpoint) {
	t.Helper()
	sequence := len(evidence.SemanticExecutions) + 1
	dispatchedAt := checkpoint.ClaimedAt.Add(time.Second)
	require.NoError(t, evidence.MarkExecutionDispatching(checkpoint.Owner, dispatchedAt))
	finishedAt := dispatchedAt.Add(time.Second)
	rawOutput := []byte(fmt.Sprintf(`{"schema_version":"ai-explanation-semantic-evaluation-output/v1","candidate":%d}`, sequence))
	receipt := mapperProviderReceiptV2(checkpoint.InvocationID, fmt.Sprintf("semantic:full:request:%d", sequence))
	require.NoError(t, evidence.CompleteSemanticExecution(checkpoint.Owner, domainevaluation.SemanticEvaluationExecution{
		ID: checkpoint.ID, CandidateID: checkpoint.CandidateID, ExecutionOrdinal: checkpoint.ExecutionOrdinal,
		InvocationID: checkpoint.InvocationID, Status: domainevaluation.ExecutionStatusSucceeded,
		StartedAt: dispatchedAt, FinishedAt: &finishedAt, ProviderCallCount: 1, ProviderReceipt: &receipt,
		RawOutput: rawOutput, NormalizedOutput: rawOutput,
		Result: &domainevaluation.SemanticEvaluationResult{
			EvaluatorVersion: "v1", Scores: domainevaluation.SemanticScores{Faithfulness: 5, CrossDimensionQuality: 5, SuggestionActionability: 5, AudienceClarity: 5, Concision: 5},
			Rationale: "output remains faithful to the frozen input",
			Decisions: []domainevaluation.SemanticDecision{{
				Type: "case_goal", Scope: domainevaluation.AssertionScopeCase, Ordinal: 1,
				Status: domainevaluation.AssertionPassed, Detail: "candidate satisfies the frozen case goal",
			}},
			OutputFingerprint: domainai.NewFingerprint(rawOutput),
		},
	}))
}

func mapperProviderReceiptV2(invocationID, requestID string) domainai.ProviderReceipt {
	return domainai.ProviderReceipt{
		InvocationID: invocationID, RequestID: requestID, Provider: "deepseek", Model: "deepseek-v4-pro",
		InputTokens: 100, OutputTokens: 200, Latency: time.Second,
	}
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

func TestEvidenceV2FailureDiagnosticsSurviveBSONAndClone(t *testing.T) {
	failure := domainevaluation.ClassifiedFailure{
		SchemaVersion: domainevaluation.FailureTaxonomySchemaVersionV1, Stage: domainevaluation.FailureStageSemanticEvaluation,
		Kind: domainevaluation.FailureKindSemanticExecution, Code: domainevaluation.SemanticProviderNoMessage,
		Retryable: true, Disposition: domainevaluation.FailureDispositionRetrySemantic, SafeMessage: "Provider 未返回结构化消息", EvidenceRefs: []string{"execution:1"},
		ProviderDiagnostics: &domainai.ProviderFailureDiagnostics{Code: "provider_output_cardinality_invalid", RequestID: "resp_test", ResponseStatus: "completed", ResponseShape: "no_message"},
	}
	require.NoError(t, failure.Validate())
	po := PromptEvaluationEvidenceV2PO{SemanticExecutions: []domainevaluation.SemanticEvaluationExecution{{Failure: &failure}}}
	raw, err := bson.Marshal(po)
	require.NoError(t, err)
	var decoded PromptEvaluationEvidenceV2PO
	require.NoError(t, bson.Unmarshal(raw, &decoded))
	require.Equal(t, failure, *decoded.SemanticExecutions[0].Failure)
	cloned := failure.Clone()
	cloned.ProviderDiagnostics.RequestID = "changed"
	require.Equal(t, "resp_test", failure.ProviderDiagnostics.RequestID)
	// Old documents have no diagnostics; the optional field remains absent.
	failure.ProviderDiagnostics = nil
	raw, err = bson.Marshal(failure)
	require.NoError(t, err)
	var document bson.M
	require.NoError(t, bson.Unmarshal(raw, &document))
	require.NotContains(t, document, "provider_diagnostics")
}

func TestEvidenceV2CancellationRoundTripsAuditAndReleasesActiveKeys(t *testing.T) {
	evidence := newMapperEvidenceV2(t)
	require.NoError(t, evidence.Cancel("user:42", "superseded release", false, evidence.Audit.CreatedAt.Add(24*time.Hour)))
	mapper := NewMapper()
	po, err := mapper.PromptEvaluationEvidenceV2ToPO(evidence)
	require.NoError(t, err)
	require.Empty(t, po.ActiveReleaseKey)
	require.Empty(t, po.ActiveExecutionOrgKey)
	raw, err := bson.Marshal(po)
	require.NoError(t, err)
	var decoded PromptEvaluationEvidenceV2PO
	require.NoError(t, bson.Unmarshal(raw, &decoded))
	restored, err := mapper.PromptEvaluationEvidenceV2ToDomain(&decoded)
	require.NoError(t, err)
	require.Equal(t, evidence.StateTransitions, restored.StateTransitions)
	var catalog evidenceV2CatalogPO
	require.NoError(t, bson.Unmarshal(raw, &catalog))
	require.Equal(t, 35, catalog.ExecutionPolicy.SlotPolicy.RequiredGenerationCases*catalog.ExecutionPolicy.SlotPolicy.RequiredCandidatesPerCase)
	require.Equal(t, "superseded release", catalog.Transitions[len(catalog.Transitions)-1].Reason)
}

func TestSemanticAdjudicationSurvivesFinalizedBSONRoundTrip(t *testing.T) {
	e := completeMapperEvidenceV2(t)
	e.GatePolicy.Version = "v2"
	fp, err := e.GatePolicy.Fingerprint()
	require.NoError(t, err)
	e.Release.GatePolicy.Version, e.Release.GatePolicy.Fingerprint = "v2", fp
	e.Release.Fingerprint, err = e.Release.ExpectedFingerprint()
	require.NoError(t, err)
	c, j := e.Slots[0].Candidate, &e.SemanticExecutions[0]
	detail := "No prohibited claims found."
	j.Result.Decisions = append(j.Result.Decisions, domainevaluation.SemanticDecision{Type: "forbidden_claims_absent", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Status: domainevaluation.AssertionFailed, Detail: detail})
	c.Assertions = append(c.Assertions, domainevaluation.AssertionReceipt{Type: "forbidden_claims_absent", Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "semantic-" + j.Result.EvaluatorVersion, Status: domainevaluation.AssertionFailed, Detail: detail})
	at := e.Audit.CreatedAt.Add(24 * time.Hour)
	for _, slot := range e.Slots {
		for _, role := range []domainevaluation.ReviewRole{domainevaluation.ReviewRoleAssessmentSemantics, domainevaluation.ReviewRoleSafetyProduct} {
			r := domainevaluation.CandidateHumanReview{CandidateID: slot.Candidate.ID, Role: role, Reviewer: "user:" + string(role), Decision: domainevaluation.ReviewDecisionApprove, Reason: "Reviewed frozen evidence", ReviewedAt: at}
			if slot.Candidate.ID == c.ID {
				r.SemanticReview = &domainevaluation.SemanticContradictionReview{PolicyVersion: domainevaluation.SemanticAdjudicationPolicyV1, ExecutionID: j.ID, OutputFingerprint: j.Result.OutputFingerprint, AssertionOrdinal: 1, OriginalDetail: detail, CandidateExcerpt: "schema_version", Reason: "Reason contradicts failed verdict; checked frozen candidate"}
			}
			require.NoError(t, e.AddHumanReview(r))
		}
	}
	require.NoError(t, e.Finalize("user:admin", "review_completed", at.Add(time.Minute)))
	require.True(t, e.GateResult.Passed)
	po, err := NewMapper().PromptEvaluationEvidenceV2ToPO(e)
	require.NoError(t, err)
	raw, err := bson.Marshal(po)
	require.NoError(t, err)
	var decoded PromptEvaluationEvidenceV2PO
	require.NoError(t, bson.Unmarshal(raw, &decoded))
	restored, err := NewMapper().PromptEvaluationEvidenceV2ToDomain(&decoded)
	require.NoError(t, err)
	require.NoError(t, restored.Validate())
	require.Equal(t, e.HumanReviews, restored.HumanReviews)
	require.Equal(t, e.GateResult, restored.GateResult)
	require.Equal(t, e.SemanticExecutions, restored.SemanticExecutions)
	require.Equal(t, e.Slots, restored.Slots)
	require.Len(t, restored.GateResult.SemanticAdjudications, 1)
}

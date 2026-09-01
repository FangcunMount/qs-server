package evaluation

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestCurrentV2PoliciesAreSelfValidatingAndFreeze140ProviderCalls(t *testing.T) {
	execution := CurrentEvaluationExecutionPolicy()
	require.NoError(t, execution.Validate())
	require.Equal(t, 140, execution.WorstCaseProviderCalls())
	_, err := execution.Fingerprint()
	require.NoError(t, err)

	gate := CurrentReleaseGatePolicy()
	require.NoError(t, gate.Validate())
	_, err = gate.Fingerprint()
	require.NoError(t, err)
}

func TestEvaluationExecutionPolicySeparatesRequiredCandidatesFromWorstCaseCalls(t *testing.T) {
	policy := validEvaluationExecutionPolicy()
	require.NoError(t, policy.Validate())
	require.Equal(t, 35, policy.RequiredCandidateCount())
	require.Equal(t, 140, policy.WorstCaseProviderCalls())
	_, err := policy.Fingerprint()
	require.NoError(t, err)

	ambiguous := policy.Clone()
	ambiguous.Recovery.ManualRecoveryStageCodes = append(ambiguous.Recovery.ManualRecoveryStageCodes, ambiguous.Recovery.AutoRetryableStageCodes[0])
	require.ErrorContains(t, ambiguous.Validate(), "ambiguous")

	unboundedQualityReplacement := policy.Clone()
	unboundedQualityReplacement.Recovery.QualityFailureReplacementAllowed = true
	require.ErrorContains(t, unboundedQualityReplacement.Validate(), "recovery invariants")
}

func TestReleaseGatePolicyFreezesDenominatorsAndQualityThresholds(t *testing.T) {
	policy := validReleaseGatePolicy()
	require.NoError(t, policy.Validate())
	_, err := policy.Fingerprint()
	require.NoError(t, err)

	excludesUnknown := policy.Clone()
	excludesUnknown.ExecutionReliability.IncludeResultUnknownInInfrastructureDenominator = false
	require.ErrorContains(t, excludesUnknown.Validate(), "reliability")

	replacesLowQuality := policy.Clone()
	replacesLowQuality.CandidateQuality.QualityFailureReplacementAllowed = true
	require.ErrorContains(t, replacesLowQuality.Validate(), "candidate quality")
}

func TestClassifiedFailureKeepsRetryReplacementAndCandidateSemanticsDistinct(t *testing.T) {
	outputFailure := ClassifiedFailure{
		SchemaVersion: FailureTaxonomySchemaVersionV1,
		Stage:         FailureStageOutputValidation, Kind: FailureKindOutputContractConformance,
		Code: "provider_output_content_contract_invalid", Disposition: FailureDispositionReplaceGeneration,
		SafeMessage: "Provider 输出不符合冻结内容契约", EvidenceRefs: []string{"generation-execution:1"},
	}
	require.NoError(t, outputFailure.Validate())
	require.True(t, outputFailure.AllowsGenerationReplacement())
	require.False(t, outputFailure.CandidateExists())

	semanticFailure := ClassifiedFailure{
		SchemaVersion: FailureTaxonomySchemaVersionV1,
		Stage:         FailureStageSemanticEvaluation, Kind: FailureKindSemanticExecution,
		Code: "semantic_output_schema_invalid", Retryable: true, Disposition: FailureDispositionRetrySemantic,
		SafeMessage: "AI 裁判输出不符合契约", EvidenceRefs: []string{"semantic-execution:1"},
	}
	require.NoError(t, semanticFailure.Validate())
	require.True(t, semanticFailure.AllowsSemanticRetry())
	require.True(t, semanticFailure.CandidateExists())

	semanticFailure.Disposition = FailureDispositionReplaceGeneration
	require.ErrorContains(t, semanticFailure.Validate(), "semantic execution")

	unknown := ClassifiedFailure{
		SchemaVersion: FailureTaxonomySchemaVersionV1,
		Stage:         FailureStageGenerationExecution, Kind: FailureKindResultUnknown,
		Code: "provider_result_unknown", ResultUnknown: true, Disposition: FailureDispositionManualAcknowledgement,
		SafeMessage: "无法确认 Provider 是否已完成调用", EvidenceRefs: []string{"checkpoint:1"},
	}
	require.NoError(t, unknown.Validate())
	require.True(t, unknown.RequiresManualAcknowledgement())
	unknown.Retryable = true
	require.ErrorContains(t, unknown.Validate(), "manual acknowledgement")
}

func TestPromptEvaluationEvidenceV2AcceptsBoundedReplacementAndSemanticRetry(t *testing.T) {
	evidence := validCollectingEvidenceV2(t)
	require.NoError(t, evidence.Validate())

	cloned := evidence.Clone()
	cloned.Slots[0].GenerationExecutionIDs[0] = "mutated"
	require.NotEqual(t, cloned.Slots[0].GenerationExecutionIDs[0], evidence.Slots[0].GenerationExecutionIDs[0])

	cherryPicked := evidence.Clone()
	cherryPicked.Slots[0].Candidate.GenerationExecutionID = cherryPicked.Slots[0].GenerationExecutionIDs[0]
	require.ErrorContains(t, cherryPicked.Validate(), "first contract-conformant")

	regeneratedAfterSemanticFailure := evidence.Clone()
	regeneratedAfterSemanticFailure.Slots[0].GenerationExecutionIDs = append(regeneratedAfterSemanticFailure.Slots[0].GenerationExecutionIDs, "generation:unexpected")
	require.Error(t, regeneratedAfterSemanticFailure.Validate())
}

func TestPromptEvaluationEvidenceV2RejectsReviewBeforeSemanticEvidence(t *testing.T) {
	evidence := validCollectingEvidenceV2(t)
	evidence.Slots[0].Candidate.AcceptedSemanticExecutionID = ""
	evidence.Slots[0].Candidate.SemanticExecutionIDs = nil
	evidence.Slots[0].Candidate.ReviewReady = false
	evidence.SemanticExecutions = nil
	evidence.HumanReviews = []CandidateHumanReview{{
		CandidateID: evidence.Slots[0].Candidate.ID,
		Role:        ReviewRoleAssessmentSemantics, Reviewer: "reviewer:1", Decision: ReviewDecisionApprove,
		ReviewedAt: evidence.Audit.CreatedAt.Add(10 * time.Minute), Reason: "完成领域语义复核",
	}}
	require.ErrorContains(t, evidence.Validate(), "without complete semantic evidence")
}

func TestPromptEvaluationEvidenceV2AddsHumanReviewBatchAtomically(t *testing.T) {
	evidence := completeEvidenceV2ForReview(t)
	evidence.HumanReviews = nil
	beforeVersion := evidence.Version()
	reviewedAt := evidence.Audit.ClosedAt.Add(time.Minute)
	firstCandidateID := evidence.Slots[0].Candidate.ID
	secondCandidateID := evidence.Slots[1].Candidate.ID

	err := evidence.AddHumanReviews([]CandidateHumanReview{
		{CandidateID: firstCandidateID, Role: ReviewRoleAssessmentSemantics, Reviewer: "user:42", Decision: ReviewDecisionApprove, ReviewedAt: reviewedAt, Reason: "语义与冻结事实一致"},
		{CandidateID: "candidate:missing", Role: ReviewRoleAssessmentSemantics, Reviewer: "user:42", Decision: ReviewDecisionApprove, ReviewedAt: reviewedAt, Reason: "不可写入的目标"},
	})
	require.ErrorContains(t, err, "unknown candidate")
	require.Empty(t, evidence.HumanReviews)
	require.Equal(t, beforeVersion, evidence.Version())

	require.NoError(t, evidence.AddHumanReviews([]CandidateHumanReview{
		{CandidateID: firstCandidateID, Role: ReviewRoleAssessmentSemantics, Reviewer: "user:42", Decision: ReviewDecisionApprove, ReviewedAt: reviewedAt, Reason: "语义与冻结事实一致"},
		{CandidateID: secondCandidateID, Role: ReviewRoleAssessmentSemantics, Reviewer: "user:42", Decision: ReviewDecisionReject, ReviewedAt: reviewedAt, Reason: "存在超出输入证据的推断"},
	}))
	require.Len(t, evidence.HumanReviews, 2)
	require.Equal(t, beforeVersion+1, evidence.Version())
}

func TestPromptEvaluationEvidenceV2StateMachineKeepsBlockedRecoveryAuditable(t *testing.T) {
	template := validCollectingEvidenceV2(t)
	caseIDs := make([]string, RequiredGenerationCaseCount)
	for index := range caseIDs {
		caseIDs[index] = fmt.Sprintf("case-%d", index+1)
	}
	createdAt := template.Audit.CreatedAt
	evidence, err := NewPromptEvaluationEvidenceV2(
		meta.ID(9902), template.Release, template.ExecutionPolicy, template.GatePolicy, caseIDs,
		"preflight-ineligible", 7, "user:42", "验证候选发布组合", createdAt,
	)
	require.NoError(t, err)
	require.Equal(t, EvidenceStatusRequested, evidence.Status)
	require.Len(t, evidence.Slots, RequiredGenerationAttempts)

	require.NoError(t, evidence.Transition(EvidenceStatusCollecting, "capacity_reserved", "system:runner", nil, createdAt.Add(time.Minute)))
	require.NoError(t, evidence.Transition(EvidenceStatusBlocked, "result_unknown_requires_review", "system:runner", []string{"checkpoint:1"}, createdAt.Add(2*time.Minute)))
	require.NoError(t, evidence.Transition(EvidenceStatusCollecting, "manual_recovery_approved", "user:88", []string{"checkpoint:1"}, createdAt.Add(3*time.Minute)))
	require.Len(t, evidence.StateTransitions, 4)
	require.Error(t, evidence.Transition(EvidenceStatusApproved, "bypass_gate", "user:88", nil, createdAt.Add(4*time.Minute)))
}

func TestPromptEvaluationEvidenceV2PreservesAndResolvesResultUnknown(t *testing.T) {
	evidence := validCollectingEvidenceV2(t)
	startedAt := evidence.Audit.CreatedAt.Add(9 * time.Minute)
	finishedAt := startedAt.Add(time.Minute)
	unknownFailure := ClassifiedFailure{
		SchemaVersion: FailureTaxonomySchemaVersionV1,
		Stage:         FailureStageGenerationExecution, Kind: FailureKindResultUnknown,
		Code: "provider_result_unknown", ResultUnknown: true, Disposition: FailureDispositionManualAcknowledgement,
		SafeMessage: "无法确认 Provider 是否已完成调用", EvidenceRefs: []string{"generation:unknown"},
	}
	evidence.GenerationExecutions = append(evidence.GenerationExecutions, CandidateGenerationExecution{
		ID: "generation:unknown", CaseID: "case-1", SlotOrdinal: 2, ExecutionOrdinal: 1, InvocationID: "generation-invocation:unknown",
		Status: ExecutionStatusResultUnknown, StartedAt: startedAt, FinishedAt: &finishedAt, ProviderCallCount: 1, Failure: &unknownFailure,
	})
	evidence.Slots[1].GenerationExecutionIDs = []string{"generation:unknown"}
	evidence.UnresolvedResultUnknownCount = 1
	require.NoError(t, evidence.Transition(EvidenceStatusBlocked, "result_unknown_requires_review", "system:runner", []string{"generation:unknown"}, finishedAt))
	require.NoError(t, evidence.Validate())

	require.NoError(t, evidence.ResolveResultUnknown(ResultUnknownResolution{
		ExecutionID: "generation:unknown", Decision: ResultUnknownAuthorizeReplacement,
		Actor: "user:88", Reason: "确认可能发生重复调用与计费，授权在冻结预算内补样",
		AcknowledgedDuplicateCallAndCostRisk: true, ResolvedAt: finishedAt.Add(time.Minute),
	}))
	require.NoError(t, evidence.Validate())
	require.Equal(t, EvidenceStatusCollecting, evidence.Status)
	require.Equal(t, ExecutionStatusResultUnknown, evidence.GenerationExecutions[len(evidence.GenerationExecutions)-1].Status)
}

func TestPromptEvaluationEvidenceV2GateRetainsRecoveredFailuresInReliabilityDenominators(t *testing.T) {
	evidence := completeEvidenceV2ForReview(t)
	evaluatedAt := evidence.Audit.CreatedAt.Add(3 * time.Hour)
	gate, err := evidence.EvaluateGate(evaluatedAt)
	require.NoError(t, err)
	require.False(t, gate.GatePasses["G3"])
	require.True(t, gate.GatePasses["G4"])
	require.True(t, gate.GatePasses["G5"])
	require.False(t, gate.Passed)
	require.Equal(t, 36, gate.Metrics[2].Denominator)
	require.Equal(t, 35, gate.Metrics[2].Numerator)

	require.NoError(t, evidence.Finalize("user:approver", "release_gate_evaluated", evaluatedAt))
	require.Equal(t, EvidenceStatusRejected, evidence.Status)
	require.NoError(t, evidence.Validate())
}

func validEvaluationExecutionPolicy() EvaluationExecutionPolicy {
	return EvaluationExecutionPolicy{
		SchemaVersion: EvaluationExecutionPolicySchemaVersionV1,
		PolicyID:      "release-evaluation-bounded-recovery",
		Version:       "v1",
		SlotPolicy: EvaluationSlotPolicy{
			RequiredGenerationCases: RequiredGenerationCaseCount, RequiredCandidatesPerCase: RequiredRepetitionsPerCase,
			RequiredPreflightCases: 1, CandidateSelection: CandidateSelectionFirstContractConformant,
		},
		Generation: GenerationExecutionBudget{MaxExecutionsPerSlot: 2, MaxExecutionsPerRun: 70},
		Semantic:   SemanticExecutionBudget{MaxExecutionsPerCandidate: 2, MaxExecutionsPerRun: 70},
		Recovery: EvaluationRecoveryPolicy{
			AutoRetryableStageCodes: []FailureSelector{
				{Stage: FailureStageGenerationExecution, Code: "provider_rate_limited"},
				{Stage: FailureStageSemanticEvaluation, Code: "semantic_provider_rate_limited"},
				{Stage: FailureStageSemanticEvaluation, Code: SemanticOutputSchemaInvalid},
			},
			ManualRecoveryStageCodes: []FailureSelector{
				{Stage: FailureStageGenerationExecution, Code: "provider_result_unknown"},
			},
			ResultUnknownRequiresManualAcknowledgement: true,
		},
	}
}

func validReleaseGatePolicy() ReleaseGatePolicy {
	return ReleaseGatePolicy{
		SchemaVersion: ReleaseGatePolicySchemaVersionV1,
		PolicyID:      "release-gates",
		Version:       "v1",
		ReleaseIdentity: ReleaseIdentityGatePolicy{
			RequiredComponents:      append([]ReleaseIdentityComponent(nil), requiredReleaseIdentityComponents...),
			RequireFingerprintMatch: true,
		},
		SampleCompleteness: SampleCompletenessGatePolicy{
			RequiredGenerationCases: RequiredGenerationCaseCount, RequiredCandidatesPerCase: RequiredRepetitionsPerCase,
			RequiredCandidateCount: RequiredGenerationAttempts, RequiredSemanticReceiptsPerCandidate: 1,
			RejectUnresolvedResultUnknown: true, RejectBudgetOverrun: true,
		},
		ExecutionReliability: ExecutionReliabilityGatePolicy{
			MinInfrastructureSuccessRate: 0.98, MinGenerationContractConformanceRate: 0.95, MinSemanticExecutionSuccessRate: 0.98,
			InfrastructureDenominator:                       "dispatched_provider_executions",
			GenerationContractDenominator:                   "definite_output_generation_executions",
			SemanticExecutionDenominator:                    "dispatched_semantic_executions",
			IncludeResultUnknownInInfrastructureDenominator: true,
		},
		CandidateQuality: CandidateQualityGatePolicy{
			MinAssertionPassesPerCase: 4, MinAssertionPassesOverall: 32,
			MinimumSemanticScores: SemanticScoreThresholds{
				Faithfulness: 4, CrossDimensionQuality: 3, SuggestionActionability: 3, AudienceClarity: 3, Concision: 3,
			},
			MinimumSemanticAverages: SemanticScoreThresholds{
				Faithfulness: 4.5, CrossDimensionQuality: 4, SuggestionActionability: 4, AudienceClarity: 4, Concision: 4,
			},
			HardAssertionFailureRejectsRelease: true,
		},
		HumanAccountability: HumanAccountabilityGatePolicy{
			RequiredRoles:               []ReviewRole{ReviewRoleAssessmentSemantics, ReviewRoleSafetyProduct},
			RequiredReviewsPerCandidate: 2, RequiredReviewCount: 70,
			RequireDistinctReviewersPerCandidate: true, RequireReason: true, AnyRejectionRejectsRelease: true,
		},
		ApprovalRule: "all_gates_must_pass",
	}
}

func validCollectingEvidenceV2(t *testing.T) PromptEvaluationEvidenceV2 {
	t.Helper()
	createdAt := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	executionPolicy := validEvaluationExecutionPolicy()
	gatePolicy := validReleaseGatePolicy()
	executionFingerprint, err := executionPolicy.Fingerprint()
	require.NoError(t, err)
	gateFingerprint, err := gatePolicy.Fingerprint()
	require.NoError(t, err)
	ref := func(id string) FrozenContractRef {
		return FrozenContractRef{ID: id, Version: "v1", Fingerprint: aiexplanation.NewFingerprint([]byte(id))}
	}
	release := EvidenceReleaseIdentity{
		Suite: ref("suite"), Prompt: ref("prompt"), Profile: ref("profile"), InputSchema: ref("input-schema"), OutputSchema: ref("output-schema"),
		GenerationRoute: ref("generation-route"), SemanticPrompt: ref("semantic-prompt"), SemanticOutputSchema: ref("semantic-output-schema"), SemanticRoute: ref("semantic-route"),
		ExecutionPolicy: FrozenContractRef{ID: executionPolicy.PolicyID, Version: executionPolicy.Version, Fingerprint: executionFingerprint},
		GatePolicy:      FrozenContractRef{ID: gatePolicy.PolicyID, Version: gatePolicy.Version, Fingerprint: gateFingerprint},
	}
	release.Fingerprint, err = release.ExpectedFingerprint()
	require.NoError(t, err)

	rawGeneration := []byte(`{"schema_version":"ai-explanation-output/v1","summary":"ok"}`)
	generationReceipt := providerReceipt("generation:2", "generation-request")
	finishedFailure := createdAt.Add(2 * time.Minute)
	finishedGeneration := createdAt.Add(4 * time.Minute)
	outputFailure := ClassifiedFailure{
		SchemaVersion: FailureTaxonomySchemaVersionV1,
		Stage:         FailureStageOutputValidation, Kind: FailureKindOutputContractConformance,
		Code: "provider_output_content_contract_invalid", Disposition: FailureDispositionReplaceGeneration,
		SafeMessage: "Provider 输出不符合冻结内容契约", EvidenceRefs: []string{"generation:1"},
	}
	generationExecutions := []CandidateGenerationExecution{
		{
			ID: "generation:1", CaseID: "case-1", SlotOrdinal: 1, ExecutionOrdinal: 1, InvocationID: "generation-invocation:1",
			Status: ExecutionStatusFailed, StartedAt: createdAt.Add(time.Minute), FinishedAt: &finishedFailure,
			ProviderCallCount: 1, RawOutput: []byte(`{"unexpected":true}`), Failure: &outputFailure,
		},
		{
			ID: "generation:2", CaseID: "case-1", SlotOrdinal: 1, ExecutionOrdinal: 2, InvocationID: generationReceipt.InvocationID,
			Status: ExecutionStatusSucceeded, StartedAt: createdAt.Add(3 * time.Minute), FinishedAt: &finishedGeneration,
			ProviderCallCount: 1, ProviderReceipt: &generationReceipt, RawOutput: rawGeneration, NormalizedOutput: rawGeneration,
			NormalizedOutputFingerprint: aiexplanation.NewFingerprint(rawGeneration),
		},
	}

	rawSemantic := []byte(`{"schema_version":"ai-explanation-semantic-evaluation-output/v1","scores":{"faithfulness":5}}`)
	semanticFailureReceipt := providerReceipt("semantic:1", "semantic-failure-request")
	semanticSuccessReceipt := providerReceipt("semantic:2", "semantic-success-request")
	finishedSemanticFailure := createdAt.Add(6 * time.Minute)
	finishedSemanticSuccess := createdAt.Add(8 * time.Minute)
	semanticFailure := ClassifiedFailure{
		SchemaVersion: FailureTaxonomySchemaVersionV1,
		Stage:         FailureStageSemanticEvaluation, Kind: FailureKindSemanticExecution,
		Code: "semantic_output_schema_invalid", Retryable: true, Disposition: FailureDispositionRetrySemantic,
		SafeMessage: "AI 裁判输出不符合契约", EvidenceRefs: []string{"semantic:1"},
	}
	semanticExecutions := []SemanticEvaluationExecution{
		{
			ID: "semantic:1", CandidateID: "candidate:1", ExecutionOrdinal: 1, InvocationID: semanticFailureReceipt.InvocationID,
			Status: ExecutionStatusFailed, StartedAt: createdAt.Add(5 * time.Minute), FinishedAt: &finishedSemanticFailure,
			ProviderCallCount: 1, ProviderReceipt: &semanticFailureReceipt, RawOutput: []byte(`{"invalid":true}`), Failure: &semanticFailure,
		},
		{
			ID: "semantic:2", CandidateID: "candidate:1", ExecutionOrdinal: 2, InvocationID: semanticSuccessReceipt.InvocationID,
			Status: ExecutionStatusSucceeded, StartedAt: createdAt.Add(7 * time.Minute), FinishedAt: &finishedSemanticSuccess,
			ProviderCallCount: 1, ProviderReceipt: &semanticSuccessReceipt, RawOutput: rawSemantic, NormalizedOutput: rawSemantic,
			Result: &SemanticEvaluationResult{
				EvaluatorVersion: "v1", Scores: SemanticScores{5, 5, 5, 5, 5}, Rationale: "输出忠实引用输入事实",
				Decisions:         []SemanticDecision{{Type: "faithfulness", Scope: AssertionScopeDefault, Ordinal: 1, Status: AssertionPassed, Detail: "引用可解析"}},
				OutputFingerprint: aiexplanation.NewFingerprint(rawSemantic),
			},
		},
	}

	slots := make([]CandidateSlot, 0, RequiredGenerationAttempts)
	for caseIndex := 1; caseIndex <= RequiredGenerationCaseCount; caseIndex++ {
		for ordinal := 1; ordinal <= RequiredRepetitionsPerCase; ordinal++ {
			slot := CandidateSlot{CaseID: fmt.Sprintf("case-%d", caseIndex), Ordinal: ordinal, Status: CandidateSlotPending}
			if caseIndex == 1 && ordinal == 1 {
				slot.Status = CandidateSlotAccepted
				slot.GenerationExecutionIDs = []string{"generation:1", "generation:2"}
				slot.Candidate = &Candidate{
					ID: "candidate:1", GenerationExecutionID: "generation:2", NormalizedOutputFingerprint: aiexplanation.NewFingerprint(rawGeneration),
					AcceptedAt: finishedGeneration, Assertions: []AssertionReceipt{{
						Type: "output_schema_valid", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "deterministic-v1", Status: AssertionPassed,
					}},
					SemanticExecutionIDs: []string{"semantic:1", "semantic:2"}, AcceptedSemanticExecutionID: "semantic:2", ReviewReady: true,
				}
			}
			slots = append(slots, slot)
		}
	}
	return PromptEvaluationEvidenceV2{
		SchemaVersion: PromptEvaluationEvidenceSchemaVersionV2,
		RunID:         meta.ID(9901), Release: release, ExecutionPolicy: executionPolicy, GatePolicy: gatePolicy,
		version: 1,
		Status:  EvidenceStatusCollecting,
		PreflightEvidence: []PreflightCaseEvidence{{
			CaseID: "preflight-ineligible", Status: PreflightEvidencePassed, EvaluatedAt: copyTime(createdAt.Add(2 * time.Second)),
			RejectionReason: "insufficient_eligible_dimensions",
			Assertions: []AssertionReceipt{
				{Type: "provider_call_count", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: AssertionPassed},
				{Type: "rejection_reason", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: AssertionPassed},
			},
		}},
		Slots: slots, GenerationExecutions: generationExecutions, SemanticExecutions: semanticExecutions,
		StateTransitions: []EvidenceStateTransition{
			{To: EvidenceStatusRequested, CauseCode: "evaluation_requested", Actor: "user:42", TransitionedAt: createdAt},
			{From: evidenceStatusPtr(EvidenceStatusRequested), To: EvidenceStatusCollecting, CauseCode: "capacity_reserved", Actor: "system:runner", TransitionedAt: createdAt.Add(time.Second)},
		},
		Audit: EvidenceRunAudit{OrganizationID: 7, RequestedBy: "user:42", RequestReason: "验证候选发布组合", CreatedAt: createdAt},
	}
}

func completeEvidenceV2ForReview(t *testing.T) PromptEvaluationEvidenceV2 {
	t.Helper()
	evidence := validCollectingEvidenceV2(t)
	createdAt := evidence.Audit.CreatedAt
	evidence.Slots[0].Candidate.Assertions = append(evidence.Slots[0].Candidate.Assertions, AssertionReceipt{
		Type: "case_goal", Scope: AssertionScopeCase, Ordinal: 1, Evaluator: "semantic-v1", Status: AssertionPassed,
	})
	for index := range evidence.Slots {
		slot := &evidence.Slots[index]
		if index > 0 {
			generationID := fmt.Sprintf("generation:complete:%d", index+1)
			semanticID := fmt.Sprintf("semantic:complete:%d", index+1)
			candidateID := fmt.Sprintf("candidate:complete:%d", index+1)
			rawGeneration := []byte(fmt.Sprintf(`{"schema_version":"ai-explanation-output/v1","summary":"candidate %d"}`, index+1))
			generationReceipt := providerReceipt(generationID, fmt.Sprintf("generation-request-%d", index+1))
			generationStartedAt := createdAt.Add(time.Duration(10+index*2) * time.Minute)
			generationFinishedAt := generationStartedAt.Add(time.Second)
			evidence.GenerationExecutions = append(evidence.GenerationExecutions, CandidateGenerationExecution{
				ID: generationID, CaseID: slot.CaseID, SlotOrdinal: slot.Ordinal, ExecutionOrdinal: 1, InvocationID: generationReceipt.InvocationID,
				Status: ExecutionStatusSucceeded, StartedAt: generationStartedAt, FinishedAt: &generationFinishedAt,
				ProviderCallCount: 1, ProviderReceipt: &generationReceipt, RawOutput: rawGeneration, NormalizedOutput: rawGeneration,
				NormalizedOutputFingerprint: aiexplanation.NewFingerprint(rawGeneration),
			})
			rawSemantic := []byte(fmt.Sprintf(`{"schema_version":"ai-explanation-semantic-evaluation-output/v1","candidate":%d}`, index+1))
			semanticReceipt := providerReceipt(semanticID, fmt.Sprintf("semantic-request-%d", index+1))
			semanticStartedAt := generationFinishedAt.Add(time.Second)
			semanticFinishedAt := semanticStartedAt.Add(time.Second)
			evidence.SemanticExecutions = append(evidence.SemanticExecutions, SemanticEvaluationExecution{
				ID: semanticID, CandidateID: candidateID, ExecutionOrdinal: 1, InvocationID: semanticReceipt.InvocationID,
				Status: ExecutionStatusSucceeded, StartedAt: semanticStartedAt, FinishedAt: &semanticFinishedAt,
				ProviderCallCount: 1, ProviderReceipt: &semanticReceipt, RawOutput: rawSemantic, NormalizedOutput: rawSemantic,
				Result: &SemanticEvaluationResult{
					EvaluatorVersion: "v1", Scores: SemanticScores{5, 5, 5, 5, 5}, Rationale: "输出忠实引用输入事实",
					Decisions:         []SemanticDecision{{Type: "case_goal", Scope: AssertionScopeCase, Ordinal: 1, Status: AssertionPassed, Detail: "满足 case 目标"}},
					OutputFingerprint: aiexplanation.NewFingerprint(rawSemantic),
				},
			})
			slot.Status = CandidateSlotAccepted
			slot.GenerationExecutionIDs = []string{generationID}
			slot.Candidate = &Candidate{
				ID: candidateID, GenerationExecutionID: generationID, NormalizedOutputFingerprint: aiexplanation.NewFingerprint(rawGeneration), AcceptedAt: generationFinishedAt,
				Assertions: []AssertionReceipt{
					{Type: "output_schema_valid", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "deterministic-v1", Status: AssertionPassed},
					{Type: "case_goal", Scope: AssertionScopeCase, Ordinal: 1, Evaluator: "semantic-v1", Status: AssertionPassed},
				},
				SemanticExecutionIDs: []string{semanticID}, AcceptedSemanticExecutionID: semanticID, ReviewReady: true,
			}
		}
	}
	require.NoError(t, evidence.Transition(EvidenceStatusAwaitingReview, "candidate_evidence_complete", "system:runner", nil, createdAt.Add(150*time.Minute)))
	reviewedAt := createdAt.Add(160 * time.Minute)
	for index, slot := range evidence.Slots {
		candidateID := slot.Candidate.ID
		require.NoError(t, evidence.AddHumanReview(CandidateHumanReview{CandidateID: candidateID, Role: ReviewRoleAssessmentSemantics, Reviewer: fmt.Sprintf("reviewer:assessment:%d", index+1), Decision: ReviewDecisionApprove, ReviewedAt: reviewedAt, Reason: "完成测评语义审核"}))
		require.NoError(t, evidence.AddHumanReview(CandidateHumanReview{CandidateID: candidateID, Role: ReviewRoleSafetyProduct, Reviewer: fmt.Sprintf("reviewer:safety:%d", index+1), Decision: ReviewDecisionApprove, ReviewedAt: reviewedAt, Reason: "完成安全与产品审核"}))
	}
	return evidence
}

func evidenceStatusPtr(value EvidenceStatus) *EvidenceStatus {
	return &value
}

func providerReceipt(invocationID, requestID string) aiexplanation.ProviderReceipt {
	return aiexplanation.ProviderReceipt{
		InvocationID: invocationID, RequestID: requestID, Provider: "deepseek", Model: "deepseek-v4-pro",
		InputTokens: 100, OutputTokens: 200, Latency: time.Second,
	}
}

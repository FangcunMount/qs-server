package evaluation

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestEvidenceV2NextActionUsesFrozenSlotOrder(t *testing.T) {
	evidence := newEmptyCollectingEvidenceV2(t)

	action, err := evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionGeneration, action.Kind)
	require.Equal(t, "case-1", action.CaseID)
	require.Equal(t, 1, action.SlotOrdinal)
	require.Equal(t, 1, action.ExecutionOrdinal)
}

func TestEvidenceV2NextActionBoundsGenerationReplacementPerSlot(t *testing.T) {
	evidence := newEmptyCollectingEvidenceV2(t)
	template := validCollectingEvidenceV2(t)
	firstFailure := template.GenerationExecutions[0]
	evidence.GenerationExecutions = append(evidence.GenerationExecutions, firstFailure)
	evidence.Slots[0].GenerationExecutionIDs = []string{firstFailure.ID}

	action, err := evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionGeneration, action.Kind)
	require.Equal(t, 2, action.ExecutionOrdinal)

	secondFailure := firstFailure
	secondFailure.ID = "generation:failed:2"
	secondFailure.InvocationID = "generation-invocation:failed:2"
	secondFailure.ExecutionOrdinal = 2
	secondFailure.StartedAt = firstFailure.StartedAt.Add(time.Minute)
	secondFailure.FinishedAt = copyTime(secondFailure.StartedAt.Add(time.Minute))
	secondFailure.Failure = failureWithEvidenceRef(*firstFailure.Failure, secondFailure.ID)
	evidence.GenerationExecutions = append(evidence.GenerationExecutions, secondFailure)
	evidence.Slots[0].GenerationExecutionIDs = append(evidence.Slots[0].GenerationExecutionIDs, secondFailure.ID)

	action, err = evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionBlock, action.Kind)
	require.Equal(t, "generation_budget_exhausted", action.CauseCode)
}

func TestEvidenceV2NextActionRetriesSemanticForSameCandidate(t *testing.T) {
	evidence := validCollectingEvidenceV2(t)
	evidence.SemanticExecutions = evidence.SemanticExecutions[:1]
	evidence.Slots[0].Candidate.SemanticExecutionIDs = evidence.Slots[0].Candidate.SemanticExecutionIDs[:1]
	evidence.Slots[0].Candidate.AcceptedSemanticExecutionID = ""
	evidence.Slots[0].Candidate.ReviewReady = false

	action, err := evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionSemantic, action.Kind)
	require.Equal(t, evidence.Slots[0].Candidate.ID, action.CandidateID)
	require.Equal(t, 2, action.ExecutionOrdinal)
	require.Equal(t, []string{"generation:1", "generation:2"}, evidence.Slots[0].GenerationExecutionIDs)
}

func TestEvidenceV2NextActionBlocksSemanticRetryNotAllowedByFrozenPolicy(t *testing.T) {
	evidence := validCollectingEvidenceV2(t)
	evidence.SemanticExecutions = evidence.SemanticExecutions[:1]
	evidence.Slots[0].Candidate.SemanticExecutionIDs = evidence.Slots[0].Candidate.SemanticExecutionIDs[:1]
	evidence.Slots[0].Candidate.AcceptedSemanticExecutionID = ""
	evidence.Slots[0].Candidate.ReviewReady = false
	evidence.ExecutionPolicy.Recovery.AutoRetryableStageCodes = nil
	executionFingerprint, err := evidence.ExecutionPolicy.Fingerprint()
	require.NoError(t, err)
	evidence.Release.ExecutionPolicy.Fingerprint = executionFingerprint
	evidence.Release.Fingerprint, err = evidence.Release.ExpectedFingerprint()
	require.NoError(t, err)

	action, err := evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionBlock, action.Kind)
	require.Equal(t, "semantic_recovery_not_allowed", action.CauseCode)
}

func TestEvidenceV2NextActionBlocksUnresolvedResultUnknown(t *testing.T) {
	evidence := newEmptyCollectingEvidenceV2(t)
	startedAt := evidence.Audit.CreatedAt.Add(time.Minute)
	finishedAt := startedAt.Add(time.Minute)
	failure := ClassifiedFailure{
		SchemaVersion: FailureTaxonomySchemaVersionV1,
		Stage:         FailureStageGenerationExecution, Kind: FailureKindResultUnknown,
		Code: "provider_result_unknown", ResultUnknown: true, Disposition: FailureDispositionManualAcknowledgement,
		SafeMessage: "无法确认 Provider 是否已完成调用", EvidenceRefs: []string{"generation:unknown"},
	}
	evidence.GenerationExecutions = []CandidateGenerationExecution{{
		ID: "generation:unknown", CaseID: "case-1", SlotOrdinal: 1, ExecutionOrdinal: 1,
		InvocationID: "generation-invocation:unknown", Status: ExecutionStatusResultUnknown,
		StartedAt: startedAt, FinishedAt: &finishedAt, ProviderCallCount: 1, Failure: &failure,
	}}
	evidence.Slots[0].GenerationExecutionIDs = []string{"generation:unknown"}
	evidence.UnresolvedResultUnknownCount = 1
	require.NoError(t, evidence.Transition(EvidenceStatusBlocked, "result_unknown_requires_review", "system:runner", []string{"generation:unknown"}, finishedAt))

	action, err := evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionBlock, action.Kind)
}

func TestEvidenceV2NextActionClosesOnlyCompleteCandidateInventory(t *testing.T) {
	evidence := completeEvidenceV2ForReview(t)
	evidence.Status = EvidenceStatusCollecting
	evidence.Audit.ClosedAt = nil
	evidence.StateTransitions = evidence.StateTransitions[:len(evidence.StateTransitions)-1]
	evidence.HumanReviews = nil

	action, err := evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionAwaitReview, action.Kind)
	require.Equal(t, "candidate_evidence_complete", action.CauseCode)
}

func TestEvidenceV2ExecutionCheckpointClaimsOnlyNextAction(t *testing.T) {
	evidence := newEmptyCollectingEvidenceV2(t)
	claimedAt := evidence.Audit.CreatedAt.Add(time.Minute)
	checkpoint := EvidenceExecutionCheckpoint{
		ID: "checkpoint:generation:1", Kind: EvidenceExecutionGeneration,
		CaseID: "case-1", SlotOrdinal: 1, ExecutionOrdinal: 1,
		Owner: "worker:1", InvocationID: "generation-invocation:1", Phase: AttemptExecutionPrepared,
		ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	}
	require.NoError(t, evidence.BeginNextExecution(checkpoint))
	require.Equal(t, int64(3), evidence.Version())

	action, err := evidence.NextAction()
	require.NoError(t, err)
	require.True(t, action.Resume)
	require.Equal(t, EvidenceNextActionGeneration, action.Kind)
	require.Equal(t, checkpoint.InvocationID, evidence.Execution().InvocationID)

	require.NoError(t, evidence.MarkExecutionDispatching("worker:1", claimedAt.Add(30*time.Second)))
	require.Equal(t, AttemptExecutionDispatching, evidence.Execution().Phase)
	require.Error(t, evidence.ReleaseExpiredPreparation(claimedAt.Add(2*time.Minute)))
}

func TestEvidenceV2CompletionKeepsGenerationAndSemanticRecoverySeparate(t *testing.T) {
	evidence := newEmptyCollectingEvidenceV2(t)
	template := validCollectingEvidenceV2(t)
	claimedAt := evidence.Audit.CreatedAt.Add(time.Minute)

	generationCheckpoint := runtimeCheckpoint(EvidenceExecutionGeneration, "execution:generation:1", "case-1", 1, "", 1, claimedAt)
	require.NoError(t, evidence.BeginNextExecution(generationCheckpoint))
	require.NoError(t, evidence.MarkExecutionDispatching(generationCheckpoint.Owner, claimedAt.Add(10*time.Second)))
	generation := template.GenerationExecutions[1]
	generation.ID = generationCheckpoint.ID
	generation.CaseID = generationCheckpoint.CaseID
	generation.SlotOrdinal = generationCheckpoint.SlotOrdinal
	generation.ExecutionOrdinal = generationCheckpoint.ExecutionOrdinal
	generation.InvocationID = generationCheckpoint.InvocationID
	generation.StartedAt = claimedAt.Add(10 * time.Second)
	generation.FinishedAt = copyTime(generation.StartedAt.Add(time.Second))
	generation.ProviderReceipt = runtimeReceipt(generation.ProviderReceipt, generation.InvocationID)
	require.NoError(t, evidence.CompleteGenerationExecution(generationCheckpoint.Owner, "candidate:runtime:1", []AssertionReceipt{{
		Type: "output_schema_valid", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "deterministic-v1", Status: AssertionPassed,
	}}, generation))
	require.Equal(t, "candidate:runtime:1", evidence.Slots[0].Candidate.ID)

	action, err := evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionSemantic, action.Kind)
	require.Equal(t, "candidate:runtime:1", action.CandidateID)

	semanticCheckpoint1 := runtimeCheckpoint(EvidenceExecutionSemantic, "execution:semantic:1", "case-1", 1, action.CandidateID, 1, claimedAt.Add(time.Minute))
	require.NoError(t, evidence.BeginNextExecution(semanticCheckpoint1))
	require.NoError(t, evidence.MarkExecutionDispatching(semanticCheckpoint1.Owner, semanticCheckpoint1.ClaimedAt.Add(10*time.Second)))
	semanticFailure := template.SemanticExecutions[0]
	semanticFailure.ID = semanticCheckpoint1.ID
	semanticFailure.CandidateID = semanticCheckpoint1.CandidateID
	semanticFailure.ExecutionOrdinal = semanticCheckpoint1.ExecutionOrdinal
	semanticFailure.InvocationID = semanticCheckpoint1.InvocationID
	semanticFailure.StartedAt = semanticCheckpoint1.ClaimedAt.Add(10 * time.Second)
	semanticFailure.FinishedAt = copyTime(semanticFailure.StartedAt.Add(time.Second))
	semanticFailure.ProviderReceipt = runtimeReceipt(semanticFailure.ProviderReceipt, semanticFailure.InvocationID)
	semanticFailure.Failure = failureWithEvidenceRef(*semanticFailure.Failure, semanticFailure.ID)
	require.NoError(t, evidence.CompleteSemanticExecution(semanticCheckpoint1.Owner, semanticFailure))
	require.Equal(t, []string{generation.ID}, evidence.Slots[0].GenerationExecutionIDs)

	action, err = evidence.NextAction()
	require.NoError(t, err)
	require.Equal(t, EvidenceNextActionSemantic, action.Kind)
	require.Equal(t, 2, action.ExecutionOrdinal)
	require.Equal(t, "candidate:runtime:1", action.CandidateID)

	semanticCheckpoint2 := runtimeCheckpoint(EvidenceExecutionSemantic, "execution:semantic:2", "case-1", 1, action.CandidateID, 2, claimedAt.Add(2*time.Minute))
	require.NoError(t, evidence.BeginNextExecution(semanticCheckpoint2))
	require.NoError(t, evidence.MarkExecutionDispatching(semanticCheckpoint2.Owner, semanticCheckpoint2.ClaimedAt.Add(10*time.Second)))
	semanticSuccess := template.SemanticExecutions[1]
	semanticSuccess.ID = semanticCheckpoint2.ID
	semanticSuccess.CandidateID = semanticCheckpoint2.CandidateID
	semanticSuccess.ExecutionOrdinal = semanticCheckpoint2.ExecutionOrdinal
	semanticSuccess.InvocationID = semanticCheckpoint2.InvocationID
	semanticSuccess.StartedAt = semanticCheckpoint2.ClaimedAt.Add(10 * time.Second)
	semanticSuccess.FinishedAt = copyTime(semanticSuccess.StartedAt.Add(time.Second))
	semanticSuccess.ProviderReceipt = runtimeReceipt(semanticSuccess.ProviderReceipt, semanticSuccess.InvocationID)
	require.NoError(t, evidence.CompleteSemanticExecution(semanticCheckpoint2.Owner, semanticSuccess))
	require.True(t, evidence.Slots[0].Candidate.ReviewReady)
	require.Equal(t, semanticSuccess.ID, evidence.Slots[0].Candidate.AcceptedSemanticExecutionID)
}

func TestEvidenceV2LastSemanticCompletionEntersAwaitingReview(t *testing.T) {
	evidence := completeEvidenceV2ForReview(t)
	evidence.Status = EvidenceStatusCollecting
	evidence.Audit.ClosedAt = nil
	evidence.StateTransitions = evidence.StateTransitions[:len(evidence.StateTransitions)-1]
	evidence.HumanReviews = nil
	lastSlot := &evidence.Slots[len(evidence.Slots)-1]
	semanticID := lastSlot.Candidate.AcceptedSemanticExecutionID
	var terminal SemanticEvaluationExecution
	for index, execution := range evidence.SemanticExecutions {
		if execution.ID == semanticID {
			terminal = execution
			evidence.SemanticExecutions = append(evidence.SemanticExecutions[:index], evidence.SemanticExecutions[index+1:]...)
			break
		}
	}
	lastSlot.Candidate.SemanticExecutionIDs = nil
	lastSlot.Candidate.AcceptedSemanticExecutionID = ""
	lastSlot.Candidate.ReviewReady = false
	claimedAt := terminal.StartedAt
	checkpoint := runtimeCheckpoint(EvidenceExecutionSemantic, terminal.ID, lastSlot.CaseID, lastSlot.Ordinal, lastSlot.Candidate.ID, 1, claimedAt)
	require.NoError(t, evidence.BeginNextExecution(checkpoint))
	require.NoError(t, evidence.MarkExecutionDispatching(checkpoint.Owner, claimedAt.Add(time.Millisecond)))
	terminal.InvocationID = checkpoint.InvocationID
	terminal.StartedAt = claimedAt.Add(time.Millisecond)
	terminal.FinishedAt = copyTime(terminal.StartedAt.Add(time.Second))
	terminal.ProviderReceipt = runtimeReceipt(terminal.ProviderReceipt, terminal.InvocationID)
	require.NoError(t, evidence.CompleteSemanticExecution(checkpoint.Owner, terminal))
	require.Equal(t, EvidenceStatusAwaitingReview, evidence.Status)
	require.NotNil(t, evidence.Audit.ClosedAt)
}

func newEmptyCollectingEvidenceV2(t *testing.T) PromptEvaluationEvidenceV2 {
	t.Helper()
	template := validCollectingEvidenceV2(t)
	caseIDs := make([]string, RequiredGenerationCaseCount)
	for index := range caseIDs {
		caseIDs[index] = fmt.Sprintf("case-%d", index+1)
	}
	createdAt := template.Audit.CreatedAt
	evidence, err := NewPromptEvaluationEvidenceV2(
		meta.ID(9910), template.Release, template.ExecutionPolicy, template.GatePolicy,
		caseIDs, "preflight-ineligible", 7, "user:42", "验证轻量 Slot 补执行", createdAt,
	)
	require.NoError(t, err)
	evidence.PreflightEvidence[0] = PreflightCaseEvidence{
		CaseID: "preflight-ineligible", Status: PreflightEvidencePassed, EvaluatedAt: copyTime(createdAt.Add(time.Second)),
		RejectionReason: "insufficient_eligible_dimensions",
		Assertions: []AssertionReceipt{
			{Type: "provider_call_count", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: AssertionPassed},
			{Type: "rejection_reason", Scope: AssertionScopeDefault, Ordinal: 1, Hard: true, Evaluator: "preflight-v1", Status: AssertionPassed},
		},
	}
	require.NoError(t, evidence.Transition(EvidenceStatusCollecting, "capacity_reserved", "system:runner", nil, createdAt.Add(2*time.Second)))
	return *evidence
}

func failureWithEvidenceRef(value ClassifiedFailure, ref string) *ClassifiedFailure {
	value.EvidenceRefs = []string{ref}
	return &value
}

func runtimeCheckpoint(kind EvidenceExecutionKind, id, caseID string, slotOrdinal int, candidateID string, executionOrdinal int, claimedAt time.Time) EvidenceExecutionCheckpoint {
	return EvidenceExecutionCheckpoint{
		ID: id, Kind: kind, CaseID: caseID, SlotOrdinal: slotOrdinal, CandidateID: candidateID, ExecutionOrdinal: executionOrdinal,
		Owner: "worker:runtime", InvocationID: id + ":invocation", Phase: AttemptExecutionPrepared,
		ClaimedAt: claimedAt, LeaseExpiresAt: claimedAt.Add(time.Minute),
	}
}

func runtimeReceipt(value *aiexplanation.ProviderReceipt, invocationID string) *aiexplanation.ProviderReceipt {
	cloned := *value
	cloned.InvocationID = invocationID
	return &cloned
}

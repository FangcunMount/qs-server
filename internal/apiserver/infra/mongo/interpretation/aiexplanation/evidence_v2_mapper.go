package aiexplanation

import (
	"fmt"
	"strconv"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

func (*Mapper) PromptEvaluationEvidenceV2ToPO(value *domainevaluation.PromptEvaluationEvidenceV2) (*PromptEvaluationEvidenceV2PO, error) {
	if value == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation evidence v2 is required")
	}
	if err := value.Validate(); err != nil {
		return nil, err
	}
	snapshot := value.Clone()
	activeReleaseKey := ""
	if evidenceV2ReleaseActive(snapshot.Status) {
		activeReleaseKey = snapshot.Release.Fingerprint.String()
	}
	activeExecutionOrgKey := ""
	if snapshot.Status == domainevaluation.EvidenceStatusCollecting {
		activeExecutionOrgKey = strconv.FormatInt(snapshot.Audit.OrganizationID, 10)
	}
	return &PromptEvaluationEvidenceV2PO{
		BaseDocument: base.BaseDocument{
			DomainID: snapshot.RunID, CreatedAt: snapshot.Audit.CreatedAt, UpdatedAt: snapshot.LastModifiedAt(),
		},
		EvidenceVersion: PromptEvaluationEvidenceVersionV2,
		SchemaVersion:   snapshot.SchemaVersion,
		Release:         snapshot.Release, ReleaseFingerprint: snapshot.Release.Fingerprint.String(),
		ExecutionPolicy: snapshot.ExecutionPolicy, GatePolicy: snapshot.GatePolicy,
		Status: string(snapshot.Status), Version: snapshot.Version(),
		PreflightEvidence: snapshot.PreflightEvidence, Slots: snapshot.Slots,
		GenerationExecutions: snapshot.GenerationExecutions, SemanticExecutions: snapshot.SemanticExecutions,
		HumanReviews: snapshot.HumanReviews, UnresolvedResultUnknownCount: snapshot.UnresolvedResultUnknownCount,
		ResultUnknownResolutions: snapshot.ResultUnknownResolutions, StateTransitions: snapshot.StateTransitions,
		GateResult: snapshot.GateResult, Audit: snapshot.Audit, Execution: evidenceV2ExecutionToPO(snapshot.Execution()),
		ActiveReleaseKey: activeReleaseKey, ActiveExecutionOrgKey: activeExecutionOrgKey,
		RequestedOrgID: snapshot.Audit.OrganizationID, ClosedAt: copyInfraTime(snapshot.Audit.ClosedAt),
		FinalizedAt: copyInfraTime(snapshot.Audit.FinalizedAt), CanceledAt: copyInfraTime(snapshot.Audit.CanceledAt),
	}, nil
}

func (*Mapper) PromptEvaluationEvidenceV2ToDomain(po *PromptEvaluationEvidenceV2PO) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	if po == nil || po.EvidenceVersion != PromptEvaluationEvidenceVersionV2 || po.SchemaVersion != domainevaluation.PromptEvaluationEvidenceSchemaVersionV2 {
		return nil, fmt.Errorf("AI explanation Prompt evaluation evidence v2 document is invalid")
	}
	value := domainevaluation.PromptEvaluationEvidenceV2{
		SchemaVersion: po.SchemaVersion, RunID: po.DomainID,
		Release: po.Release, ExecutionPolicy: po.ExecutionPolicy, GatePolicy: po.GatePolicy,
		Status: domainevaluation.EvidenceStatus(po.Status), PreflightEvidence: po.PreflightEvidence, Slots: po.Slots,
		GenerationExecutions: po.GenerationExecutions, SemanticExecutions: po.SemanticExecutions,
		HumanReviews: po.HumanReviews, UnresolvedResultUnknownCount: po.UnresolvedResultUnknownCount,
		ResultUnknownResolutions: po.ResultUnknownResolutions, StateTransitions: po.StateTransitions,
		GateResult: po.GateResult, Audit: po.Audit,
	}
	restored, err := domainevaluation.RestorePromptEvaluationEvidenceV2(value, po.Version, evidenceV2ExecutionFromPO(po.Execution))
	if err != nil {
		return nil, err
	}
	expectedReleaseKey := ""
	if evidenceV2ReleaseActive(restored.Status) {
		expectedReleaseKey = restored.Release.Fingerprint.String()
	}
	expectedOrgKey := ""
	if restored.Status == domainevaluation.EvidenceStatusCollecting {
		expectedOrgKey = strconv.FormatInt(restored.Audit.OrganizationID, 10)
	}
	if po.ReleaseFingerprint != restored.Release.Fingerprint.String() || po.ActiveReleaseKey != expectedReleaseKey ||
		po.ActiveExecutionOrgKey != expectedOrgKey || po.RequestedOrgID != restored.Audit.OrganizationID ||
		!po.CreatedAt.Equal(restored.Audit.CreatedAt) || !po.UpdatedAt.Equal(restored.LastModifiedAt()) ||
		!sameInfraTime(po.ClosedAt, restored.Audit.ClosedAt) || !sameInfraTime(po.FinalizedAt, restored.Audit.FinalizedAt) ||
		!sameInfraTime(po.CanceledAt, restored.Audit.CanceledAt) {
		return nil, fmt.Errorf("AI explanation Prompt evaluation evidence v2 indexed projection is inconsistent")
	}
	return restored, nil
}

func evidenceV2ReleaseActive(status domainevaluation.EvidenceStatus) bool {
	switch status {
	case domainevaluation.EvidenceStatusRequested, domainevaluation.EvidenceStatusCollecting,
		domainevaluation.EvidenceStatusBlocked, domainevaluation.EvidenceStatusAwaitingReview:
		return true
	default:
		return false
	}
}

func evidenceV2ExecutionToPO(value *domainevaluation.EvidenceExecutionCheckpoint) *EvaluationV2ExecutionCheckpointPO {
	if value == nil {
		return nil
	}
	return &EvaluationV2ExecutionCheckpointPO{
		ID: value.ID, Kind: string(value.Kind), CaseID: value.CaseID, SlotOrdinal: value.SlotOrdinal,
		CandidateID: value.CandidateID, ExecutionOrdinal: value.ExecutionOrdinal,
		Owner: value.Owner, InvocationID: value.InvocationID, Phase: string(value.Phase),
		ClaimedAt: value.ClaimedAt, LeaseExpiresAt: value.LeaseExpiresAt,
		DispatchStartedAt: copyInfraTime(value.DispatchStartedAt),
	}
}

func evidenceV2ExecutionFromPO(value *EvaluationV2ExecutionCheckpointPO) *domainevaluation.EvidenceExecutionCheckpoint {
	if value == nil {
		return nil
	}
	return &domainevaluation.EvidenceExecutionCheckpoint{
		ID: value.ID, Kind: domainevaluation.EvidenceExecutionKind(value.Kind), CaseID: value.CaseID,
		SlotOrdinal: value.SlotOrdinal, CandidateID: value.CandidateID, ExecutionOrdinal: value.ExecutionOrdinal,
		Owner: value.Owner, InvocationID: value.InvocationID, Phase: domainevaluation.AttemptExecutionPhase(value.Phase),
		ClaimedAt: value.ClaimedAt, LeaseExpiresAt: value.LeaseExpiresAt,
		DispatchStartedAt: copyInfraTime(value.DispatchStartedAt),
	}
}

func copyInfraTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func sameInfraTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

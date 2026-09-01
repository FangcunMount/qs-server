package events

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

const PromptEvaluationAggregateType = "AIExplanationPromptEvaluation"

const PromptEvaluationEvidenceVersionV2 = eventpayload.AIExplanationPromptEvaluationEvidenceVersionV2

type PromptEvaluationStepEvent = event.Event[eventpayload.AIExplanationPromptEvaluationStepRequestedData]

func (Factory) PromptEvaluationRecheck(
	value *domainevaluation.PromptEvaluationRecheck,
	eventID string,
	occurredAt time.Time,
) (event.DomainEvent, error) {
	eventID = strings.TrimSpace(eventID)
	if value == nil || value.Status() != domainevaluation.RecheckStatusQueued || eventID == "" || len(eventID) > 256 ||
		occurredAt.IsZero() || value.RequestedOrgID() <= 0 || strings.TrimSpace(value.RequestedBy()) == "" {
		return nil, fmt.Errorf("queued audited AI explanation Prompt evaluation recheck and event identity are required")
	}
	return PromptEvaluationStepEvent{
		BaseEvent: event.BaseEvent{
			ID: eventID, EventTypeValue: eventcatalog.AIExplanationPromptEvaluationStepRequested,
			OccurredAtValue: occurredAt, AggregateTypeValue: "AIExplanationPromptEvaluationRecheck",
			AggregateIDValue: value.ID().String(),
		},
		Data: eventpayload.AIExplanationPromptEvaluationStepRequestedData{
			OrgID: value.RequestedOrgID(), RunID: value.SourceRunID().String(), CaseID: value.SourceCaseID(),
			Attempt: value.SourceAttempt(), RecheckID: value.ID().String(), RequestedBy: value.RequestedBy(), RequestedAt: occurredAt,
		},
	}, nil
}

func (Factory) PromptEvaluationStep(
	runRecord *domainevaluation.PromptEvaluationRun,
	caseID string,
	attempt int,
	eventID string,
	occurredAt time.Time,
) (event.DomainEvent, error) {
	return PromptEvaluationStep(runRecord, caseID, attempt, eventID, occurredAt)
}

// PromptEvaluationStep builds a caller-addressed durable wake-up. eventID must
// be stable for automatic start/continuation events; an explicitly audited
// manual recovery may provide a new request ID.
func PromptEvaluationStep(
	runRecord *domainevaluation.PromptEvaluationRun,
	caseID string,
	attempt int,
	eventID string,
	occurredAt time.Time,
) (event.DomainEvent, error) {
	eventID = strings.TrimSpace(eventID)
	caseID = strings.TrimSpace(caseID)
	if runRecord == nil || runRecord.Status() != domainevaluation.StatusCollecting || eventID == "" || len(eventID) > 256 ||
		caseID == "" || attempt < 1 || occurredAt.IsZero() || runRecord.RequestedOrgID() <= 0 ||
		strings.TrimSpace(runRecord.RequestedBy()) == "" {
		return nil, fmt.Errorf("collecting audited AI explanation Prompt evaluation and event identity are required")
	}
	targetMatches := runRecord.HasAttempt(caseID, attempt)
	if execution := runRecord.Execution(); execution != nil {
		targetMatches = targetMatches || execution.CaseID == caseID && execution.Attempt == attempt
	}
	if nextCaseID, nextAttempt, ok := runRecord.NextPendingGenerationAttempt(); ok {
		targetMatches = targetMatches || nextCaseID == caseID && nextAttempt == attempt
	}
	if !targetMatches {
		return nil, fmt.Errorf("AI explanation Prompt evaluation event target is outside current progress")
	}
	return PromptEvaluationStepEvent{
		BaseEvent: event.BaseEvent{
			ID: eventID, EventTypeValue: eventcatalog.AIExplanationPromptEvaluationStepRequested,
			OccurredAtValue: occurredAt, AggregateTypeValue: PromptEvaluationAggregateType,
			AggregateIDValue: runRecord.ID().String(),
		},
		Data: eventpayload.AIExplanationPromptEvaluationStepRequestedData{
			OrgID: runRecord.RequestedOrgID(), RunID: runRecord.ID().String(), CaseID: caseID, Attempt: attempt,
			RequestedBy: runRecord.RequestedBy(), RequestedAt: occurredAt,
		},
	}, nil
}

func (Factory) PromptEvaluationStepV2(
	value *domainevaluation.PromptEvaluationEvidenceV2,
	action domainevaluation.EvidenceNextAction,
	eventID string,
	occurredAt time.Time,
) (event.DomainEvent, error) {
	return PromptEvaluationStepV2(value, action, eventID, occurredAt)
}

// PromptEvaluationStepV2 binds one durable wake-up to the exact domain-selected
// Generation or Semantic action. Legacy attempt is deliberately left zero: it
// cannot identify both kinds without conflating their idempotency domains.
func PromptEvaluationStepV2(
	value *domainevaluation.PromptEvaluationEvidenceV2,
	action domainevaluation.EvidenceNextAction,
	eventID string,
	occurredAt time.Time,
) (event.DomainEvent, error) {
	eventID = strings.TrimSpace(eventID)
	if value == nil || value.Status != domainevaluation.EvidenceStatusCollecting || eventID == "" || len(eventID) > 256 ||
		occurredAt.IsZero() || value.Audit.OrganizationID <= 0 || strings.TrimSpace(value.Audit.RequestedBy) == "" ||
		(action.Kind != domainevaluation.EvidenceNextActionGeneration && action.Kind != domainevaluation.EvidenceNextActionSemantic) {
		return nil, fmt.Errorf("collecting audited AI explanation Prompt evaluation v2 Provider action is required")
	}
	next, err := value.NextAction()
	if err != nil {
		return nil, err
	}
	if !samePromptEvaluationV2Action(next, action) {
		return nil, fmt.Errorf("AI explanation Prompt evaluation v2 event target is not the next frozen action")
	}
	return PromptEvaluationStepEvent{
		BaseEvent: event.BaseEvent{
			ID: eventID, EventTypeValue: eventcatalog.AIExplanationPromptEvaluationStepRequested,
			OccurredAtValue: occurredAt, AggregateTypeValue: PromptEvaluationAggregateType,
			AggregateIDValue: value.RunID.String(),
		},
		Data: eventpayload.AIExplanationPromptEvaluationStepRequestedData{
			OrgID: value.Audit.OrganizationID, RunID: value.RunID.String(), CaseID: action.CaseID,
			EvidenceVersion: PromptEvaluationEvidenceVersionV2, ExecutionKind: string(action.Kind),
			SlotOrdinal: action.SlotOrdinal, CandidateID: action.CandidateID, ExecutionOrdinal: action.ExecutionOrdinal,
			RequestedBy: value.Audit.RequestedBy, RequestedAt: occurredAt,
		},
	}, nil
}

func PromptEvaluationStepEventID(runID, caseID string, attempt int) string {
	return fmt.Sprintf("ai-prompt-evaluation:%s:%s:%d", strings.TrimSpace(runID), strings.TrimSpace(caseID), attempt)
}

func PromptEvaluationStepV2EventID(runID string, action domainevaluation.EvidenceNextAction) string {
	address := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%d", action.Kind, strings.TrimSpace(action.CaseID),
		action.SlotOrdinal, strings.TrimSpace(action.CandidateID), action.ExecutionOrdinal)
	digest := sha256.Sum256([]byte(address))
	return fmt.Sprintf("ai-prompt-evaluation:v2:%s:%x", strings.TrimSpace(runID), digest[:16])
}

func PromptEvaluationRecoveryEventID(runID, requestID string) string {
	return fmt.Sprintf("ai-prompt-evaluation-recovery:%s:%s", strings.TrimSpace(runID), strings.TrimSpace(requestID))
}

func PromptEvaluationRecheckEventID(recheckID string) string {
	return "ai-prompt-evaluation-recheck:" + strings.TrimSpace(recheckID)
}

func samePromptEvaluationV2Action(left, right domainevaluation.EvidenceNextAction) bool {
	return left.Kind == right.Kind && left.CaseID == right.CaseID && left.SlotOrdinal == right.SlotOrdinal &&
		left.CandidateID == right.CandidateID && left.ExecutionOrdinal == right.ExecutionOrdinal && left.Resume == right.Resume
}

package events

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

const PromptEvaluationAggregateType = "AIExplanationPromptEvaluation"

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

func PromptEvaluationStepEventID(runID, caseID string, attempt int) string {
	return fmt.Sprintf("ai-prompt-evaluation:%s:%s:%d", strings.TrimSpace(runID), strings.TrimSpace(caseID), attempt)
}

func PromptEvaluationRecoveryEventID(runID, requestID string) string {
	return fmt.Sprintf("ai-prompt-evaluation-recovery:%s:%s", strings.TrimSpace(runID), strings.TrimSpace(requestID))
}

func PromptEvaluationRecheckEventID(recheckID string) string {
	return "ai-prompt-evaluation-recheck:" + strings.TrimSpace(recheckID)
}

package interpretation

import (
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
)

const (
	EventTypeReportGeneratedOutcome = EventTypeReportGenerated
	EventTypeReportFailedOutcome    = EventTypeReportFailed
)

type ReportGeneratedOutcomeData = eventoutcome.ReportGeneratedPayload
type ReportGeneratedOutcomeEvent = event.Event[ReportGeneratedOutcomeData]
type ReportFailedOutcomeData = eventoutcome.ReportFailedPayload
type ReportFailedOutcomeEvent = event.Event[ReportFailedOutcomeData]
type InterpretationRetryRequestedData = eventoutcome.InterpretationRetryRequestedPayload
type InterpretationRetryRequestedEvent = event.Event[InterpretationRetryRequestedData]

// ReportGeneratedEventInput captures the complete immutable trace of a
// generated report. GenerationID, rather than InterpretReport ID, is the aggregate
// identity because retries belong to the Generation lifecycle.
type ReportGeneratedEventInput struct {
	OrgID                int64
	GenerationID         string
	RunID                string
	ReportID             string
	AssessmentID         string
	OutcomeID            string
	TesteeID             uint64
	Attempt              uint
	ReportType           string
	TemplateVersion      string
	BuilderIdentity      string
	ContentSchemaVersion string
	Model                EventModelIdentity
	PrimaryScore         *EventScoreValue
	Level                *EventResultLevel
	GeneratedAt          time.Time
}

func NewInterpretationReportGeneratedEvent(input ReportGeneratedEventInput) ReportGeneratedOutcomeEvent {
	return event.New(EventTypeReportGeneratedOutcome, AggregateType, input.GenerationID,
		ReportGeneratedOutcomeData{
			OrgID: input.OrgID, GenerationID: input.GenerationID, RunID: input.RunID, ReportID: input.ReportID,
			AssessmentID: input.AssessmentID, OutcomeID: input.OutcomeID, TesteeID: input.TesteeID, Attempt: input.Attempt,
			ReportType: input.ReportType, TemplateVersion: input.TemplateVersion, BuilderIdentity: input.BuilderIdentity,
			ContentSchemaVersion: input.ContentSchemaVersion, Model: input.Model, PrimaryScore: input.PrimaryScore,
			Level: input.Level, GeneratedAt: input.GeneratedAt,
		},
	)
}

type ReportFailedEventInput struct {
	OrgID           int64
	GenerationID    string
	RunID           string
	AssessmentID    string
	OutcomeID       string
	TesteeID        uint64
	Attempt         uint
	ReportType      string
	TemplateVersion string
	FailureKind     string
	FailureCode     string
	Retryable       bool
	SafeReason      string
	FailedAt        time.Time
	RetryDecision   *eventoutcome.RetryDecisionPayload
}

func NewInterpretationReportFailedEvent(input ReportFailedEventInput) ReportFailedOutcomeEvent {
	return event.New(EventTypeReportFailedOutcome, AggregateType, input.GenerationID,
		ReportFailedOutcomeData{
			OrgID: input.OrgID, GenerationID: input.GenerationID, RunID: input.RunID, AssessmentID: input.AssessmentID,
			OutcomeID: input.OutcomeID, TesteeID: input.TesteeID, Attempt: input.Attempt, ReportType: input.ReportType,
			TemplateVersion: input.TemplateVersion, FailureKind: input.FailureKind, FailureCode: input.FailureCode,
			Retryable: input.Retryable, SafeReason: input.SafeReason, FailedAt: input.FailedAt,
			RetryDecision: cloneRetryDecisionPayload(input.RetryDecision),
		},
	)
}

func cloneRetryDecisionPayload(value *eventoutcome.RetryDecisionPayload) *eventoutcome.RetryDecisionPayload {
	if value == nil {
		return nil
	}
	copied := *value
	if value.NextAttemptAt != nil {
		next := *value.NextAttemptAt
		copied.NextAttemptAt = &next
	}
	return &copied
}

type RetryRequestedEventInput struct {
	OrgID                                        int64
	GenerationID, RunID, AssessmentID, OutcomeID string
	TesteeID                                     uint64
	ExpectedAttempt                              int
	AttemptOrigin, ActionRequestID, Mode         string
	RequestedAt                                  time.Time
}

func NewInterpretationRetryRequestedEvent(input RetryRequestedEventInput) InterpretationRetryRequestedEvent {
	eventID := fmt.Sprintf("interpret-retry:%s:%d:%s", input.GenerationID, input.ExpectedAttempt, input.AttemptOrigin)
	if input.ActionRequestID != "" {
		eventID += ":" + input.ActionRequestID
	}
	return InterpretationRetryRequestedEvent{BaseEvent: event.BaseEvent{
		ID: eventID, EventTypeValue: EventTypeRetryRequested, OccurredAtValue: input.RequestedAt,
		AggregateTypeValue: AggregateType, AggregateIDValue: input.GenerationID,
	}, Data: InterpretationRetryRequestedData{
		OrgID: input.OrgID, GenerationID: input.GenerationID, RunID: input.RunID,
		AssessmentID: input.AssessmentID, OutcomeID: input.OutcomeID, TesteeID: input.TesteeID,
		ExpectedAttempt: input.ExpectedAttempt, AttemptOrigin: input.AttemptOrigin,
		ActionRequestID: input.ActionRequestID, Mode: input.Mode, RequestedAt: input.RequestedAt,
	}}
}

package interpretation

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	EventTypeReportGeneratedOutcome = EventTypeReportGenerated
	EventTypeReportFailedOutcome    = EventTypeReportFailed
)

type ReportGeneratedOutcomeData = eventoutcome.ReportGeneratedPayload
type ReportGeneratedOutcomeEvent = event.Event[ReportGeneratedOutcomeData]
type ReportFailedOutcomeData = eventoutcome.ReportFailedPayload
type ReportFailedOutcomeEvent = event.Event[ReportFailedOutcomeData]

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
}

func NewInterpretationReportFailedEvent(input ReportFailedEventInput) ReportFailedOutcomeEvent {
	return event.New(EventTypeReportFailedOutcome, AggregateType, input.GenerationID,
		ReportFailedOutcomeData{
			OrgID: input.OrgID, GenerationID: input.GenerationID, RunID: input.RunID, AssessmentID: input.AssessmentID,
			OutcomeID: input.OutcomeID, TesteeID: input.TesteeID, Attempt: input.Attempt, ReportType: input.ReportType,
			TemplateVersion: input.TemplateVersion, FailureKind: input.FailureKind, FailureCode: input.FailureCode,
			Retryable: input.Retryable, SafeReason: input.SafeReason, FailedAt: input.FailedAt,
		},
	)
}

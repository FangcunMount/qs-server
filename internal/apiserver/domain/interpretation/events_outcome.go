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

// ReportGeneratedOutcomeData 是结果-enriched report generated 事件载荷。
type ReportGeneratedOutcomeData = eventoutcome.ReportGeneratedPayload
type ReportGeneratedOutcomeEvent = event.Event[ReportGeneratedOutcomeData]
type ReportFailedOutcomeData = eventoutcome.ReportFailedPayload
type ReportFailedOutcomeEvent = event.Event[ReportFailedOutcomeData]

func NewInterpretationReportGeneratedEvent(
	orgID int64,
	reportID string,
	assessmentID string,
	outcomeID string,
	testeeID uint64,
	attempt uint,
	model EventModelIdentity,
	primary *EventScoreValue,
	level *EventResultLevel,
	generatedAt time.Time,
) ReportGeneratedOutcomeEvent {
	return event.New(EventTypeReportGeneratedOutcome, AggregateType, reportID,
		ReportGeneratedOutcomeData{
			OrgID:        orgID,
			ReportID:     reportID,
			AssessmentID: assessmentID,
			OutcomeID:    outcomeID,
			TesteeID:     testeeID,
			Attempt:      attempt,
			Model:        model,
			PrimaryScore: primary,
			Level:        level,
			GeneratedAt:  generatedAt,
		},
	)
}

func NewInterpretationReportFailedEvent(
	orgID int64,
	reportID string,
	assessmentID string,
	outcomeID string,
	testeeID uint64,
	attempt uint,
	reason string,
	failedAt time.Time,
) ReportFailedOutcomeEvent {
	return event.New(EventTypeReportFailedOutcome, AggregateType, reportID,
		ReportFailedOutcomeData{
			OrgID:        orgID,
			ReportID:     reportID,
			AssessmentID: assessmentID,
			OutcomeID:    outcomeID,
			TesteeID:     testeeID,
			Attempt:      attempt,
			Reason:       reason,
			FailedAt:     failedAt,
		},
	)
}

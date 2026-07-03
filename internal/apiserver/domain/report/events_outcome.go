package report

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const EventTypeGeneratedOutcome = eventcatalog.ReportGeneratedOutcome

// ReportGeneratedOutcomeData is the outcome-enriched report generated event payload.
type ReportGeneratedOutcomeData = eventoutcome.ReportGeneratedPayload

type ReportGeneratedOutcomeEvent = event.Event[ReportGeneratedOutcomeData]

// NewReportGeneratedOutcomeEvent creates an outcome-enriched report generated event.
func NewReportGeneratedOutcomeEvent(
	reportID string,
	assessmentID string,
	testeeID uint64,
	model EventModelIdentity,
	primary *EventScoreValue,
	level *EventResultLevel,
	generatedAt time.Time,
) ReportGeneratedOutcomeEvent {
	return event.New(EventTypeGeneratedOutcome, AggregateType, reportID,
		ReportGeneratedOutcomeData{
			ReportID:     reportID,
			AssessmentID: assessmentID,
			TesteeID:     testeeID,
			Model:        model,
			PrimaryScore: primary,
			Level:        level,
			GeneratedAt:  generatedAt,
		},
	)
}

// Deprecated: use EventTypeGeneratedOutcome.
const EventTypeGeneratedV2 = EventTypeGeneratedOutcome

// Deprecated: use ReportGeneratedOutcomeData.
type ReportGeneratedV2Data = ReportGeneratedOutcomeData

// Deprecated: use ReportGeneratedOutcomeEvent.
type ReportGeneratedV2Event = ReportGeneratedOutcomeEvent

// Deprecated: use NewReportGeneratedOutcomeEvent.
func NewReportGeneratedV2Event(
	reportID string,
	assessmentID string,
	testeeID uint64,
	model EventModelIdentity,
	primary *EventScoreValue,
	level *EventResultLevel,
	generatedAt time.Time,
) ReportGeneratedOutcomeEvent {
	return NewReportGeneratedOutcomeEvent(
		reportID, assessmentID, testeeID, model, primary, level, generatedAt,
	)
}

package report

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const EventTypeGeneratedV2 = eventcatalog.ReportGeneratedV2

// ReportGeneratedV2Data is the v2 report generated event payload.
type ReportGeneratedV2Data = eventoutcome.ReportGeneratedPayload

type ReportGeneratedV2Event = event.Event[ReportGeneratedV2Data]

// NewReportGeneratedV2Event creates a v2 report generated event.
func NewReportGeneratedV2Event(
	reportID string,
	assessmentID string,
	testeeID uint64,
	model EventModelIdentity,
	primary *EventScoreValue,
	level *EventResultLevel,
	generatedAt time.Time,
) ReportGeneratedV2Event {
	return event.New(EventTypeGeneratedV2, AggregateType, reportID,
		ReportGeneratedV2Data{
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

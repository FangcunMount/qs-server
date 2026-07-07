package interpretation

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const EventTypeGeneratedOutcome = EventTypeGenerated

// ReportGeneratedOutcomeData 是结果-enriched report generated 事件载荷。
type ReportGeneratedOutcomeData = eventoutcome.ReportGeneratedPayload

type ReportGeneratedOutcomeEvent = event.Event[ReportGeneratedOutcomeData]

// NewReportGeneratedOutcomeEvent 创建结果-enriched report generated event。
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

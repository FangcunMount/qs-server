package report

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const EventTypeGeneratedV2 = eventcatalog.ReportGeneratedV2

// ReportGeneratedV2Data is the v2 report generated event payload.
type ReportGeneratedV2Data struct {
	ReportID     string             `json:"report_id"`
	AssessmentID string             `json:"assessment_id"`
	TesteeID     uint64             `json:"testee_id"`
	Model        EventModelIdentity `json:"model"`
	PrimaryScore *EventScoreValue   `json:"primary_score,omitempty"`
	Level        *EventResultLevel  `json:"level,omitempty"`
	GeneratedAt  time.Time          `json:"generated_at"`
}

// IsHighRisk reports whether the outcome should trigger high-risk workflows.
func (d ReportGeneratedV2Data) IsHighRisk() bool {
	if d.Level != nil && IsHighSeverity(d.Level.Severity) {
		return true
	}
	if d.Level != nil && IsRiskLevelCode(d.Level.Code) {
		return RiskLevel(d.Level.Code) == RiskLevelHigh || RiskLevel(d.Level.Code) == RiskLevelSevere
	}
	return false
}

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

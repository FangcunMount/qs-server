package interpretation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

// Batch I0 freezes the two terminal facts. pending/generating remain query
// states and must not become durable public events during the three-object
// migration.
func TestInterpretationDurableEventsAreTerminalReportFacts(t *testing.T) {
	if EventTypeReportGenerated != eventcatalog.InterpretationReportGenerated {
		t.Fatalf("generated event = %q", EventTypeReportGenerated)
	}
	if EventTypeReportFailed != eventcatalog.InterpretationReportFailed {
		t.Fatalf("failed event = %q", EventTypeReportFailed)
	}
}

func TestReportTerminalEventsCarryStableOutcomeCorrelation(t *testing.T) {
	at := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	max := 100.0
	generated := NewInterpretationReportGeneratedEvent(
		11, "report-7", "assessment-3", "outcome-9", 42, 2,
		EventModelIdentity{Kind: "scale", Code: "SDS", Version: "1.0.0"},
		&EventScoreValue{Kind: "raw_total", Value: 42, Label: "total", Max: &max},
		&EventResultLevel{Code: "high", Label: "high", Severity: "high"},
		at,
	)
	if generated.EventType() != EventTypeReportGenerated || generated.AggregateType() != AggregateType || generated.AggregateID() != "report-7" {
		t.Fatalf("generated event identity = type:%q aggregate:%q/%q", generated.EventType(), generated.AggregateType(), generated.AggregateID())
	}
	if payload := generated.Payload(); payload.OrgID != 11 || payload.OutcomeID != "outcome-9" || payload.ReportID != "report-7" || payload.AssessmentID != "assessment-3" || payload.TesteeID != 42 || payload.Attempt != 2 || payload.Model.Code != "SDS" || payload.PrimaryScore == nil || payload.PrimaryScore.Value != 42 || payload.PrimaryScore.Max == nil || *payload.PrimaryScore.Max != max || payload.Level == nil || payload.Level.Severity != "high" || !payload.GeneratedAt.Equal(at) {
		t.Fatalf("generated payload = %#v", payload)
	}

	failed := NewInterpretationReportFailedEvent(11, "report-7", "assessment-3", "outcome-9", 42, 2, "template unavailable", at)
	if failed.EventType() != EventTypeReportFailed || failed.AggregateType() != AggregateType || failed.AggregateID() != "report-7" {
		t.Fatalf("failed event identity = type:%q aggregate:%q/%q", failed.EventType(), failed.AggregateType(), failed.AggregateID())
	}
	if payload := failed.Payload(); payload.OrgID != 11 || payload.OutcomeID != "outcome-9" || payload.ReportID != "report-7" || payload.AssessmentID != "assessment-3" || payload.TesteeID != 42 || payload.Attempt != 2 || payload.Reason != "template unavailable" || !payload.FailedAt.Equal(at) {
		t.Fatalf("failed payload = %#v", payload)
	}
}

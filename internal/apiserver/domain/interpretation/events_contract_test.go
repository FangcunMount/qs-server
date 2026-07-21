package interpretation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
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
	generated := NewInterpretationReportGeneratedEvent(ReportGeneratedEventInput{
		OrgID: 11, GenerationID: "generation-5", RunID: "run-2", ReportID: "report-7", AssessmentID: "assessment-3", OutcomeID: "outcome-9", TesteeID: 42, Attempt: 2,
		ReportType: "standard", TemplateVersion: "v2", BuilderIdentity: "factor-scoring", ContentSchemaVersion: "report-content/v2",
		Model: EventModelIdentity{Kind: "scale", Code: "SDS", Version: "1.0.0"}, PrimaryScore: &EventScoreValue{Kind: "raw_total", Value: 42, Label: "total", Max: &max},
		Level: &EventResultLevel{Code: "high", Label: "high", Severity: "high"}, GeneratedAt: at,
	})
	if generated.EventType() != EventTypeReportGenerated || generated.AggregateType() != AggregateType || generated.AggregateID() != "generation-5" {
		t.Fatalf("generated event identity = type:%q aggregate:%q/%q", generated.EventType(), generated.AggregateType(), generated.AggregateID())
	}
	if payload := generated.Payload(); payload.OrgID != 11 || payload.GenerationID != "generation-5" || payload.RunID != "run-2" || payload.OutcomeID != "outcome-9" || payload.ReportID != "report-7" || payload.ReportType != "standard" || payload.TemplateVersion != "v2" || payload.BuilderIdentity != "factor-scoring" || payload.ContentSchemaVersion != "report-content/v2" || payload.AssessmentID != "assessment-3" || payload.TesteeID != 42 || payload.Attempt != 2 || payload.Model.Code != "SDS" || payload.PrimaryScore == nil || payload.PrimaryScore.Value != 42 || payload.PrimaryScore.Max == nil || *payload.PrimaryScore.Max != max || payload.Level == nil || payload.Level.Severity != "high" || !payload.GeneratedAt.Equal(at) {
		t.Fatalf("generated payload = %#v", payload)
	}

	failed := NewInterpretationReportFailedEvent(ReportFailedEventInput{
		OrgID: 11, GenerationID: "generation-5", RunID: "run-2", AssessmentID: "assessment-3", OutcomeID: "outcome-9", TesteeID: 42, Attempt: 2,
		ReportType: "standard", TemplateVersion: "v2", FailureKind: "template", FailureCode: "not_found", Retryable: true,
		SafeReason: "template unavailable", FailedAt: at,
		RetryDecision: &eventoutcome.RetryDecisionPayload{
			Disposition: "manual_required", Retryable: true, Attempt: 2, PolicyVersion: "business-retry/v1",
		},
	})
	if failed.EventType() != EventTypeReportFailed || failed.AggregateType() != AggregateType || failed.AggregateID() != "generation-5" {
		t.Fatalf("failed event identity = type:%q aggregate:%q/%q", failed.EventType(), failed.AggregateType(), failed.AggregateID())
	}
	if payload := failed.Payload(); payload.OrgID != 11 || payload.GenerationID != "generation-5" || payload.RunID != "run-2" || payload.OutcomeID != "outcome-9" || payload.ReportType != "standard" || payload.TemplateVersion != "v2" || payload.FailureKind != "template" || payload.FailureCode != "not_found" || !payload.Retryable || payload.SafeReason != "template unavailable" || payload.AssessmentID != "assessment-3" || payload.TesteeID != 42 || payload.Attempt != 2 || !payload.FailedAt.Equal(at) || payload.RetryDecision == nil || payload.RetryDecision.Disposition != "manual_required" || payload.RetryDecision.PolicyVersion != "business-retry/v1" {
		t.Fatalf("failed payload = %#v", payload)
	}
}

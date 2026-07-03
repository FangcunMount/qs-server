package statistics

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func TestFootprintDurableStagingPolicyDisablesHighFrequencyEvents(t *testing.T) {
	policy := NewFootprintDurableStagingPolicy(DefaultDisabledHighFrequencyFootprintEvents())
	InstallFootprintDurableStagingPolicy(policy)
	t.Cleanup(func() { InstallFootprintDurableStagingPolicy(nil) })

	if FootprintEventAllowed(eventcatalog.FootprintAnswerSheetSubmitted) {
		t.Fatal("answersheet footprint should be disabled")
	}
	if FootprintEventAllowed(eventcatalog.FootprintReportGenerated) {
		t.Fatal("report footprint should be disabled")
	}
	if FootprintEventAllowed(eventcatalog.FootprintEntryOpened) {
		t.Fatal("entry_opened footprint should be disabled")
	}
	if FootprintEventAllowed(eventcatalog.FootprintIntakeConfirmed) {
		t.Fatal("intake_confirmed footprint should be disabled")
	}
	if FootprintEventAllowed(eventcatalog.FootprintAssessmentCreated) {
		t.Fatal("assessment_created footprint should be disabled")
	}
}

func TestFilterFootprintStagingEventsRemovesDisabledTypes(t *testing.T) {
	InstallFootprintDurableStagingPolicy(NewFootprintDurableStagingPolicy([]string{
		eventcatalog.FootprintReportGenerated,
	}))
	t.Cleanup(func() { InstallFootprintDurableStagingPolicy(nil) })

	events := []event.DomainEvent{
		event.New(eventcatalog.FootprintReportGenerated, "BehaviorFootprint", "1", struct{}{}),
		event.New(eventcatalog.ReportGeneratedOutcome, "Report", "1", struct{}{}),
	}
	filtered := FilterFootprintStagingEvents(events)
	if len(filtered) != 1 || filtered[0].EventType() != eventcatalog.ReportGeneratedOutcome {
		t.Fatalf("filtered = %#v, want only report.generated.v2", filtered)
	}
}

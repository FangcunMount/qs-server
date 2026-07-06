package statistics

import (
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpolicy"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// FootprintDurableStagingPolicy is the durable-staging policy for footprint
// events. The mechanism is a neutral outbox concern; this package only owns the
// footprint-specific default disabled list and keeps a stable API for callers.
type FootprintDurableStagingPolicy = outboxpolicy.Policy

// DefaultDisabledHighFrequencyFootprintEvents lists footprint events moved to scan projection.
func DefaultDisabledHighFrequencyFootprintEvents() []string {
	return []string{
		eventcatalog.FootprintEntryOpened,
		eventcatalog.FootprintIntakeConfirmed,
		eventcatalog.FootprintTesteeProfileCreated,
		eventcatalog.FootprintCareRelationshipEstablished,
		eventcatalog.FootprintAnswerSheetSubmitted,
		eventcatalog.FootprintAssessmentCreated,
		eventcatalog.FootprintReportGenerated,
	}
}

// NewFootprintDurableStagingPolicy builds a policy from disabled event types.
func NewFootprintDurableStagingPolicy(disabledEventTypes []string) *FootprintDurableStagingPolicy {
	return outboxpolicy.NewPolicy(disabledEventTypes)
}

// InstallFootprintDurableStagingPolicy sets the process-wide footprint staging policy.
func InstallFootprintDurableStagingPolicy(policy *FootprintDurableStagingPolicy) {
	outboxpolicy.Install(policy)
}

// FootprintEventAllowed checks the installed footprint staging policy.
func FootprintEventAllowed(eventType string) bool {
	return outboxpolicy.Allowed(eventType)
}

// FilterFootprintStagingEvents removes footprint events blocked by the staging policy.
func FilterFootprintStagingEvents(events []event.DomainEvent) []event.DomainEvent {
	return outboxpolicy.Filter(events)
}
